//go:build linux

package player

import (
	"ape-player/internal/diagnostics"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

type Track struct {
	ID               int    `json:"id"`
	Type             string `json:"type"`
	Title            string `json:"title"`
	Lang             string `json:"lang"`
	Selected         bool   `json:"selected"`
	External         bool   `json:"external"`
	FFIndex          int    `json:"ff-index"`
	ExternalFilename string `json:"external-filename"`
	Codec            string `json:"codec"`
}
type State struct {
	Width, Height              int
	DroppedFrames              int64
	Position, Duration, Volume float64
	Paused, Buffering, Muted   bool
	Tracks                     []Track
	CacheRanges                []CacheRange
	HWDec                      string
	VideoCodec, AudioCodec     string
}
type Event struct {
	Kind  string
	State State
	Err   error
}
type param struct {
	Type int32
	Data unsafe.Pointer
}
type mpvEvent struct {
	ID       int32
	Error    int32
	Userdata uint64
	Data     unsafe.Pointer
}
type logMessage struct {
	Prefix, Level, Text uintptr
	LogLevel            int32
}
type endFile struct {
	Reason int32
	Error  int32
}
type api struct {
	requestLogs    func(uintptr, string) int32
	create         func() uintptr
	initialize     func(uintptr) int32
	option         func(uintptr, string, string) int32
	command        func(uintptr, **byte) int32
	property       func(uintptr, string) uintptr
	free           func(uintptr)
	wait           func(uintptr, float64) uintptr
	terminate      func(uintptr)
	renderCreate   func(*uintptr, uintptr, *param) int32
	render         func(uintptr, *param) int32
	renderFree     func(uintptr)
	renderUpdate   func(uintptr) uint64
	renderCallback func(uintptr, uintptr, uintptr)
	errorString    func(int32) string
}

var lib api
var loadOnce sync.Once
var loadErr error
var getProc, updateCallback uintptr
var clients sync.Map
var nextID atomic.Uint64

func load() {
	loadOnce.Do(func() {
		h, e := purego.Dlopen("libmpv.so.2", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if e != nil {
			loadErr = errors.New("缺少 libmpv.so.2，请安装 mpv")
			return
		}
		bindings := map[string]any{"mpv_request_log_messages": &lib.requestLogs, "mpv_create": &lib.create, "mpv_initialize": &lib.initialize, "mpv_set_option_string": &lib.option, "mpv_command": &lib.command, "mpv_get_property_string": &lib.property, "mpv_free": &lib.free, "mpv_wait_event": &lib.wait, "mpv_terminate_destroy": &lib.terminate, "mpv_render_context_create": &lib.renderCreate, "mpv_render_context_render": &lib.render, "mpv_render_context_free": &lib.renderFree, "mpv_render_context_update": &lib.renderUpdate, "mpv_render_context_set_update_callback": &lib.renderCallback, "mpv_error_string": &lib.errorString}
		for n, p := range bindings {
			sym, e := purego.Dlsym(h, n)
			if e != nil {
				loadErr = e
				return
			}
			purego.RegisterFunc(p, sym)
		}
		gl, e := purego.Dlopen("libGL.so.1", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if e != nil {
			loadErr = e
			return
		}
		var resolve func(*byte) uintptr
		purego.RegisterLibFunc(&resolve, gl, "glXGetProcAddressARB")
		getProc = purego.NewCallback(func(_ uintptr, name *byte) uintptr { return resolve(name) })
		updateCallback = purego.NewCallback(func(id uintptr) {
			if v, ok := clients.Load(id); ok {
				p := v.(*Player)
				select {
				case p.wake <- struct{}{}:
				default:
				}
			}
		})
	})
}

type Player struct {
	frames                   atomic.Uint64
	handle                   uintptr
	id                       uintptr
	renderContext, glContext uintptr // accessed only by the UI render thread
	Events                   chan Event
	wake                     chan struct{}
	done                     chan struct{}
	stopped                  chan struct{}
	commands                 chan []string
	closing                  atomic.Bool
	failed                   bool
	ready                    bool
	redraw                   func()
}

func New(redraw func()) (*Player, error) {
	if err := ConfigureNumericLocale(); err != nil {
		diagnostics.Log("mpv.create_error", "error", err.Error())
		return nil, err
	}
	load()
	if loadErr != nil {
		diagnostics.Log("mpv.create_error", "error", loadErr.Error())
		return nil, loadErr
	}
	h := lib.create()
	if h == 0 {
		diagnostics.Log("mpv.create_error", "error", "mpv_create returned NULL")
		return nil, errors.New("无法创建播放器，详情见播放诊断日志")
	}
	for k, v := range map[string]string{"vo": "libmpv", "hwdec": "auto-safe", "terminal": "no", "input-default-bindings": "no", "input-vo-keyboard": "no", "osc": "no", "config": "no", "idle": "yes", "keep-open": "no", "network-timeout": "20"} {
		if code := lib.option(h, k, v); code < 0 {
			lib.terminate(h)
			return nil, fmt.Errorf("mpv %s: %s", k, lib.errorString(code))
		}
	}
	if code := lib.initialize(h); code < 0 {
		diagnostics.Log("mpv.initialize_error", "code", code, "error", lib.errorString(code))
		lib.terminate(h)
		return nil, errors.New(lib.errorString(code))
	}
	lib.requestLogs(h, "warn")
	p := &Player{handle: h, id: uintptr(nextID.Add(1)), Events: make(chan Event, 64), wake: make(chan struct{}, 1), done: make(chan struct{}), stopped: make(chan struct{}), commands: make(chan []string, 64), redraw: redraw}
	diagnostics.Log("mpv.initialized", "version", p.get("mpv-version"), "ffmpeg", p.get("ffmpeg-version"))
	clients.Store(p.id, p)
	go p.loop()
	go func() {
		for {
			select {
			case <-p.done:
				return
			case <-p.wake:
				p.redraw()
			}
		}
	}()
	return p, nil
}
func (p *Player) emit(e Event) {
	select {
	case p.Events <- e:
	case <-p.done:
	}
}
func (p *Player) Command(args ...string) {
	if p.closing.Load() {
		return
	}
	select {
	case p.commands <- args:
	default:
		p.emit(Event{Kind: "error", Err: errors.New("播放操作过于频繁，请稍后重试")})
	}
}
func (p *Player) command(args []string) {
	b := make([][]byte, len(args))
	ptrs := make([]*byte, len(args)+1)
	for i, s := range args {
		b[i] = append([]byte(s), 0)
		ptrs[i] = &b[i][0]
	}
	code := lib.command(p.handle, &ptrs[0])
	runtime.KeepAlive(b)
	if code < 0 {
		diagnostics.Log("mpv.command_error", "command", args[0], "code", code, "error", lib.errorString(code))
		p.emit(Event{Kind: "error", Err: fmt.Errorf("播放操作失败：%s", lib.errorString(code))})
	}
}
func (p *Player) get(n string) string {
	v := lib.property(p.handle, n)
	if v == 0 {
		return ""
	}
	defer lib.free(v)
	return cString(v)
}
func cString(p uintptr) string {
	if p == 0 {
		return ""
	}
	b := []byte{}
	for i := uintptr(0); i < 1<<20; i++ {
		v := *(*byte)(unsafe.Add(nativePointer(p), i))
		if v == 0 {
			break
		}
		b = append(b, v)
	}
	return string(b)
}
func (p *Player) state() State {
	var s State
	fmt.Sscan(p.get("frame-drop-count"), &s.DroppedFrames)
	fmt.Sscan(p.get("dwidth"), &s.Width)
	fmt.Sscan(p.get("dheight"), &s.Height)
	fmt.Sscan(p.get("time-pos"), &s.Position)
	fmt.Sscan(p.get("duration"), &s.Duration)
	fmt.Sscan(p.get("volume"), &s.Volume)
	s.Paused = p.get("pause") == "yes"
	s.Muted = p.get("mute") == "yes"
	s.Buffering = p.get("paused-for-cache") == "yes"
	s.HWDec = p.get("hwdec-current")
	s.VideoCodec = p.get("video-format")
	s.AudioCodec = p.get("audio-codec-name")
	_ = json.Unmarshal([]byte(p.get("track-list")), &s.Tracks)
	s.CacheRanges = parseCacheRanges(p.get("demuxer-cache-state"))
	return s
}
func (p *Player) loop() {
	defer close(p.stopped)
	defer close(p.Events)
	defer lib.terminate(p.handle)
	last := time.Now()
	for {
		select {
		case <-p.done:
			return
		case args := <-p.commands:
			p.command(args)
		default:
		}
		ptr := lib.wait(p.handle, .05)
		if ptr != 0 {
			e := *(*mpvEvent)(nativePointer(ptr))
			switch e.ID {
			case 2:
				if e.Data != nil {
					msg := *(*logMessage)(e.Data)
					diagnostics.Log("mpv.message", "prefix", cString(msg.Prefix), "level", cString(msg.Level), "text", cString(msg.Text))
				}
			case 8:
				state := p.state()
				diagnostics.Log("mpv.loaded", "video_codec", state.VideoCodec, "audio_codec", state.AudioCodec, "duration", state.Duration, "tracks", len(state.Tracks))
				p.emit(Event{Kind: "loaded", State: state})
			case 21: // Playback restart: decoder metadata is available after loading/seeking.
				state := p.state()
				diagnostics.Log("mpv.video", "codec", state.VideoCodec, "width", state.Width, "height", state.Height, "hwdec", state.HWDec)

			case 7:
				if e.Data != nil {
					d := *(*endFile)(e.Data)
					diagnostics.Log("mpv.end", "reason", d.Reason, "code", d.Error, "error", lib.errorString(d.Error))
					if d.Reason == 0 {
						p.emit(Event{Kind: "ended", State: p.state()})
					} else if d.Reason == 4 {
						p.emit(Event{Kind: "playback-error", Err: fmt.Errorf("媒体播放失败：%s（%d），详情见播放诊断日志", lib.errorString(d.Error), d.Error)})
					}
				}
			}
		}
		if time.Since(last) > 250*time.Millisecond {
			p.emit(Event{Kind: "state", State: p.state()})
			last = time.Now()
		}
	}
}

// Render and ReleaseGraphics are called exclusively with the window's GL context current.
func (p *Player) Render(context uintptr, fbo uint32, w, h int) {
	if p.failed || p.closing.Load() {
		return
	}
	if p.renderContext != 0 && context != p.glContext {
		p.failed = true
		p.emit(Event{Kind: "error", Err: errors.New("图形上下文已重建，请重新打开视频")})
		return
	}
	if p.renderContext == 0 {
		init := struct{ GetProc, Context uintptr }{getProc, 0}
		typ := append([]byte("opengl"), 0)
		params := []param{{1, unsafe.Pointer(&typ[0])}, {2, unsafe.Pointer(&init)}, {0, nil}}
		// Supply the native display so VA-API can interoperate without copying frames.
		if display, kind := nativeDisplay(); display != 0 {
			params = append(params[:2], param{kind, nativePointer(display)}, param{})
		}
		code := lib.renderCreate(&p.renderContext, p.handle, &params[0])
		runtime.KeepAlive(typ)
		runtime.KeepAlive(init)
		if code < 0 {
			p.failed = true
			p.emit(Event{Kind: "error", Err: fmt.Errorf("GPU 播放初始化失败：%s", lib.errorString(code))})
			return
		}
		p.glContext = context
		lib.renderCallback(p.renderContext, updateCallback, p.id)
		if !p.ready {
			p.ready = true
			p.emit(Event{Kind: "ready"})
		}
	}
	p.frames.Add(1)
	lib.renderUpdate(p.renderContext)
	target := struct{ FBO, W, H, Format int32 }{int32(fbo), int32(w), int32(h), 0}
	// MyGO samples this offscreen texture with its first row at the UI top.
	// FLIP_Y is needed for a default window framebuffer, not this texture FBO.
	flip := int32(0)
	block := int32(0)
	params := []param{{3, unsafe.Pointer(&target)}, {4, unsafe.Pointer(&flip)}, {12, unsafe.Pointer(&block)}, {0, nil}}
	if code := lib.render(p.renderContext, &params[0]); code < 0 {
		p.failed = true
		p.emit(Event{Kind: "error", Err: errors.New("GPU 渲染失败")})
	}
}
func (p *Player) ReleaseGraphics() {
	if p.renderContext != 0 {
		lib.renderCallback(p.renderContext, 0, 0)
		lib.renderFree(p.renderContext)
		p.renderContext = 0
	}
}
func (p *Player) Close() {
	if p.closing.Swap(true) {
		return
	}
	clients.Delete(p.id)
	close(p.done)
}
func (p *Player) Done() <-chan struct{} { return p.stopped }
func (p *Player) Load(raw string, headers map[string]string, start float64, volume float64, audio, subtitle string) {
	diagnostics.Log("mpv.load", "url", diagnostics.URL(raw), "start", start)
	userAgent := "Go-http-client/1.1"
	for k, v := range headers {
		diagnostics.Protect(v)
		if strings.EqualFold(k, "User-Agent") {
			userAgent = v
		}
	}
	p.Command("set", "user-agent", userAgent)
	p.Command("set", "http-header-fields", headerFields(headers))
	p.Command("set", "alang", audio)
	p.Command("set", "slang", subtitle)
	p.Command("set", "volume", fmt.Sprint(volume))
	p.Command("set", "start", fmt.Sprintf("%.3f", start))
	p.Command("loadfile", raw, "replace")
}

// mpv's list option escapes backslashes and commas within each header.
func headerFields(headers map[string]string) string {
	escape := strings.NewReplacer("\\", "\\\\", ",", "\\,")
	var fields []string
	for k, v := range headers {
		if strings.EqualFold(k, "User-Agent") || strings.ContainsAny(k+v, "\r\n") {
			continue
		}
		fields = append(fields, escape.Replace(k+": "+v))
	}
	sort.Strings(fields)
	return strings.Join(fields, ",")
}
func nativeDisplay() (uintptr, int32) {
	libGDK, e := purego.Dlopen("libgdk-3.so.0", purego.RTLD_NOW)
	if e != nil {
		return 0, 0
	}
	defer purego.Dlclose(libGDK)
	var get func() uintptr
	purego.RegisterLibFunc(&get, libGDK, "gdk_display_get_default")
	display := get()
	if display == 0 {
		return 0, 0
	}
	// Runtime type checks avoid calling a Wayland accessor on an X11 display.
	obj, e := purego.Dlopen("libgobject-2.0.so.0", purego.RTLD_NOW)
	if e != nil {
		return 0, 0
	}
	defer purego.Dlclose(obj)
	var check func(uintptr, uintptr) bool
	purego.RegisterLibFunc(&check, obj, "g_type_check_instance_is_a")
	for _, v := range []struct {
		typ, access string
		kind        int32
	}{{"gdk_wayland_display_get_type", "gdk_wayland_display_get_wl_display", 9}, {"gdk_x11_display_get_type", "gdk_x11_display_get_xdisplay", 8}} {
		sym, e := purego.Dlsym(libGDK, v.typ)
		if e != nil {
			continue
		}
		var typ func() uintptr
		purego.RegisterFunc(&typ, sym)
		if check(display, typ()) {
			var access func(uintptr) uintptr
			purego.RegisterLibFunc(&access, libGDK, v.access)
			return access(display), v.kind
		}
	}
	return 0, 0
}

// TrackID maps container stream indices, never confusing Emby indices with mpv IDs.
func TrackID(tracks []Track, kind string, index int) (int, bool) {
	for _, t := range tracks {
		if t.Type == kind && !t.External && t.FFIndex == index {
			return t.ID, true
		}
	}
	return 0, false
}

// CloseUnused is called on the UI thread when no GL renderer ever attached.
func (p *Player) CloseUnused() {
	if p.renderContext == 0 {
		p.Close()
	}
}

func (p *Player) FrameCount() uint64 { return p.frames.Load() }

// Native addresses returned by libmpv/GDK refer to C-owned memory, never Go
// allocations. Preserve their bits without manufacturing Go pointer arithmetic.
func nativePointer(address uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&address))
}
