package emby

import (
	"ape-player/internal/diagnostics"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

var ErrUnauthorized = errors.New("登录已过期，请重新登录")
var ErrInvalidCredentials = errors.New("账号或密码错误，请重新输入")

type Client struct {
	Base                    *url.URL
	HTTP                    *http.Client
	Token, UserID, DeviceID string
}

func New(raw, token, user, device string, timeout int) (*Client, error) {
	u, e := url.Parse(strings.TrimSpace(raw))
	if e != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("请输入完整的 HTTP(S) 服务器地址，不含账号、查询参数或片段")
	}
	if timeout <= 0 {
		timeout = 20
	}
	diagnostics.Protect(token)
	return &Client{Base: u, Token: token, UserID: user, DeviceID: device, HTTP: &http.Client{Timeout: time.Duration(timeout) * time.Second}}, nil
}
func (c *Client) URL(p string, q url.Values) string {
	u := *c.Base
	u.Path = strings.TrimRight(u.Path, "/") + "/" + strings.TrimLeft(p, "/")
	u.RawQuery = q.Encode()
	return u.String()
}
func (c *Client) Authorization() string {
	return fmt.Sprintf(`MediaBrowser Client="Ape Player", Device="Linux", DeviceId="%s", Version="0.1.0"`, c.DeviceID)
}
func (c *Client) Do(ctx context.Context, method, p string, q url.Values, body, out any) error {
	var r io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return e
		}
		r = bytes.NewReader(b)
	}
	req, e := http.NewRequestWithContext(ctx, method, c.URL(p, q), r)
	if e != nil {
		return e
	}
	req.Header.Set("X-Emby-Authorization", c.Authorization())
	if c.Token != "" {
		req.Header.Set("X-Emby-Token", c.Token)
	}
	req.Header.Set("Content-Type", "application/json")
	res, e := c.HTTP.Do(req)
	if e != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("无法连接服务器，请检查地址和网络")
	}
	defer res.Body.Close()
	if res.StatusCode == 401 {
		return ErrUnauthorized
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("服务器请求失败（HTTP %d）", res.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(res.Body, 16<<20)).Decode(out)
	}
	return nil
}
func (c *Client) Login(ctx context.Context, name, pw string) (Auth, error) {
	var a Auth
	e := c.Do(ctx, "POST", "Users/AuthenticateByName", nil, map[string]string{"Username": name, "Pw": pw}, &a)
	if errors.Is(e, ErrUnauthorized) {
		return Auth{}, ErrInvalidCredentials
	}
	if e == nil && (a.AccessToken == "" || a.User.ID == "") {
		e = errors.New("服务器返回的登录信息不完整")
	}
	return a, e
}
func (c *Client) Logout(ctx context.Context) error {
	return c.Do(ctx, "POST", "Sessions/Logout", nil, nil, nil)
}
func (c *Client) Validate(ctx context.Context) error {
	return c.Do(ctx, "GET", "Users/"+url.PathEscape(c.UserID), nil, nil, &map[string]any{})
}
func (c *Client) List(ctx context.Context, endpoint string, q url.Values) (Items, error) {
	var a Items
	e := c.Do(ctx, "GET", endpoint, q, nil, &a)
	return a, e
}
func (c *Client) Item(ctx context.Context, id string) (Item, error) {
	var a Item
	e := c.Do(ctx, "GET", "Users/"+c.UserID+"/Items/"+id, nil, nil, &a)
	return a, e
}
func (c *Client) Favorite(ctx context.Context, id string, on bool) error {
	m := "DELETE"
	if on {
		m = "POST"
	}
	return c.Do(ctx, m, "Users/"+c.UserID+"/FavoriteItems/"+id, nil, nil, nil)
}
func (c *Client) Played(ctx context.Context, id string, on bool) error {
	m := "DELETE"
	if on {
		m = "POST"
	}
	return c.Do(ctx, m, "Users/"+c.UserID+"/PlayedItems/"+id, nil, nil, nil)
}
func (c *Client) Image(ctx context.Context, item Item) ([]byte, error) {
	q := url.Values{"maxWidth": {"360"}, "quality": {"85"}}
	if tag := item.ImageTags["Primary"]; tag != "" {
		q.Set("tag", tag)
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", c.URL("Items/"+item.ID+"/Images/Primary", q), nil)
	req.Header.Set("X-Emby-Token", c.Token)
	res, e := c.HTTP.Do(req)
	if e != nil {
		return nil, e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, errors.New("没有封面")
	}
	return io.ReadAll(io.LimitReader(res.Body, 4<<20))
}

// Resolve preserves server-provided paths and query strings, including reverse-proxy prefixes.
func (c *Client) Resolve(raw string) string {
	u, e := url.Parse(raw)
	if e != nil {
		return ""
	}
	if u.IsAbs() {
		return u.String()
	}
	if strings.HasPrefix(raw, "/") {
		return c.Base.ResolveReference(u).String()
	}
	b := *c.Base
	b.Path = strings.TrimRight(b.Path, "/") + "/"
	return b.ResolveReference(u).String()
}
func (c *Client) Prepare(ctx context.Context, item Item, mediaID string, bitrate int, start int64, audio, sub *int, forceTranscode bool) (Playback, error) {
	if bitrate <= 0 {
		bitrate = 200000000
	}
	profile := map[string]any{"Name": "Ape Player", "MaxStreamingBitrate": bitrate, "MaxStaticBitrate": 200000000, "DirectPlayProfiles": []any{map[string]any{"Type": "Video", "Container": "mkv,mp4,mov,avi,webm,ts,m2ts,mpeg,mpg,wmv,flv", "VideoCodec": "h264,hevc,vp8,vp9,av1,mpeg2video,mpeg4,vc1", "AudioCodec": "aac,ac3,eac3,dts,truehd,flac,opus,vorbis,mp3,pcm_s16le"}}, "TranscodingProfiles": []any{map[string]any{"Type": "Video", "Context": "Streaming", "Protocol": "hls", "Container": "ts", "VideoCodec": "h264", "AudioCodec": "aac", "MaxAudioChannels": "2", "MinSegments": 1, "BreakOnNonKeyFrames": true}}, "SubtitleProfiles": []any{map[string]any{"Format": "srt", "Method": "External"}, map[string]any{"Format": "ass", "Method": "External"}, map[string]any{"Format": "ssa", "Method": "External"}, map[string]any{"Format": "vtt", "Method": "External"}, map[string]any{"Format": "pgssub", "Method": "Embed"}, map[string]any{"Format": "dvdsub", "Method": "Embed"}}}
	if !forceTranscode {
		delete(profile, "TranscodingProfiles")
	}
	diagnostics.Log("playback.prepare", "item", item.ID, "transcoding", forceTranscode, "bitrate", bitrate)
	body := map[string]any{"UserId": c.UserID, "DeviceProfile": profile, "MaxStreamingBitrate": bitrate, "StartTimeTicks": start, "IsPlayback": true, "AutoOpenLiveStream": true, "EnableDirectPlay": !forceTranscode, "EnableDirectStream": !forceTranscode, "EnableTranscoding": forceTranscode}
	if mediaID != "" {
		body["MediaSourceId"] = mediaID
	}
	if audio != nil {
		body["AudioStreamIndex"] = *audio
	}
	if sub != nil {
		body["SubtitleStreamIndex"] = *sub
	}
	var info PlaybackInfo
	if e := c.Do(ctx, "POST", "Items/"+item.ID+"/PlaybackInfo", nil, body, &info); e != nil {
		diagnostics.Log("playback.negotiation_error", "error", e.Error())
		return Playback{}, e
	}
	diagnostics.Log("playback.response", "sources", len(info.MediaSources), "error_code", info.ErrorCode)
	if info.ErrorCode != "" || len(info.MediaSources) == 0 {
		return Playback{}, errors.New("服务器未提供可播放的媒体源")
	}
	s := info.MediaSources[0]
	if mediaID != "" {
		for _, v := range info.MediaSources {
			if v.ID == mediaID {
				s = v
				break
			}
		}
	}
	if item.RunTimeTicks <= 0 {
		item.RunTimeTicks = s.RunTimeTicks
	}
	s.MediaStreams = mergeStreams(s.MediaStreams, itemStreams(item, s.ID))
	if audio == nil {
		audio = s.DefaultAudioStreamIndex
	}
	if sub == nil {
		sub = s.DefaultSubtitleStreamIndex
	}
	p := Playback{Item: item, Source: s, SessionID: info.PlaySessionID, StartTicks: start, AudioIndex: audio, SubtitleIndex: sub}
	diagnostics.Log("playback.source", "media", s.ID, "container", s.Container, "protocol", s.Protocol, "direct_play", s.SupportsDirectPlay, "direct_stream", s.SupportsDirectStream, "transcoding", s.SupportsTranscoding, "has_direct_url", s.DirectStreamUrl != "")
	for _, stream := range s.MediaStreams {
		diagnostics.Log("playback.stream", "index", stream.Index, "type", stream.Type, "codec", stream.Codec)
	}
	switch {
	case !forceTranscode && (s.SupportsDirectPlay || s.SupportsDirectStream):
		p.Method = "DirectPlay"
		if !s.SupportsDirectPlay {
			p.Method = "DirectStream"
		}
		if s.DirectStreamUrl != "" {
			p.URL = c.Resolve(s.DirectStreamUrl)
		} else {
			p.URL = c.URL("Videos/"+item.ID+"/stream", url.Values{"Static": {"true"}, "MediaSourceId": {s.ID}, "DeviceId": {c.DeviceID}, "PlaySessionId": {info.PlaySessionID}})
		}
	case forceTranscode && s.TranscodingUrl != "":
		p.Method = "Transcode"
		p.URL = c.Resolve(s.TranscodingUrl)
	default:
		diagnostics.Log("playback.unavailable", "transcoding_requested", forceTranscode)
		return Playback{}, errors.New("服务器未提供可直放的媒体源，请查看播放诊断日志")
	}
	u, err := url.Parse(p.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return Playback{}, errors.New("服务器返回了无效的播放地址")
	}
	sameOrigin := strings.EqualFold(u.Scheme, c.Base.Scheme) && strings.EqualFold(u.Host, c.Base.Host)
	if s.AddApiKeyToDirectStreamUrl && p.Method != "Transcode" && sameOrigin {
		q := u.Query()
		q.Set("api_key", c.Token)
		u.RawQuery = q.Encode()
		p.URL = u.String()
	}
	p.Headers = map[string]string{"User-Agent": "Go-http-client/1.1"}
	if sameOrigin {
		p.Headers["X-Emby-Token"] = c.Token
		p.Headers["X-Emby-Authorization"] = c.Authorization()
	}
	for k, v := range s.RequiredHttpHeaders {
		if strings.ContainsAny(k+v, "\r\n") {
			return Playback{}, errors.New("服务器返回了无效的媒体请求头")
		}
		p.Headers[http.CanonicalHeaderKey(k)] = v
		diagnostics.Protect(v)
	}
	diagnostics.Log("playback.selected", "method", p.Method, "url", diagnostics.URL(p.URL), "server_auth", sameOrigin)

	return p, nil
}
func (c *Client) Report(ctx context.Context, event string, p Playback, pos float64, paused bool) error {
	err := c.Do(ctx, "POST", "Sessions/Playing"+event, nil, map[string]any{"ItemId": p.Item.ID, "MediaSourceId": p.Source.ID, "PlaySessionId": p.SessionID, "PositionTicks": int64(pos * TicksPerSecond), "IsPaused": paused, "CanSeek": true, "PlayMethod": p.Method, "AudioStreamIndex": p.AudioIndex, "SubtitleStreamIndex": p.SubtitleIndex}, nil)
	diagnostics.Log("emby.report", "event", "Playing"+event, "item_id", p.Item.ID, "position", pos, "paused", paused, "error", err)
	return err
}
func (c *Client) StopTranscode(ctx context.Context, p Playback) error {
	if p.Method != "Transcode" {
		return nil
	}
	return c.Do(ctx, "DELETE", "Videos/ActiveEncodings", url.Values{"DeviceId": {c.DeviceID}, "PlaySessionId": {p.SessionID}}, nil, nil)
}
func (c *Client) SubtitleURL(p Playback, s Stream) string {
	raw := c.Resolve(s.DeliveryUrl)
	if s.DeliveryUrl == "" {
		ext := s.Codec
		if ext == "subrip" {
			ext = "srt"
		}
		raw = c.URL(path.Join("Videos", p.Item.ID, p.Source.ID, "Subtitles", strconv.Itoa(s.Index), "Stream."+ext), nil)
	}
	// The video may be a signed CDN URL with no Emby headers. Authenticate a
	// separate subtitle request only when it points back to this Emby server.
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if strings.EqualFold(u.Scheme, c.Base.Scheme) && strings.EqualFold(u.Host, c.Base.Host) && c.Token != "" {
		q := u.Query()
		if q.Get("api_key") == "" {
			q.Set("api_key", c.Token)
			u.RawQuery = q.Encode()
		}
	}
	return u.String()
}
