package app

import (
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Capture the compositor's actual window pixels: mpv screenshots and MyGO's
// CPU CapturePage bypass the GL texture composition where flipping can occur.
func TestGPUVideoOrientation(t *testing.T) {
	if os.Getenv("APE_GPU_ORIENTATION_TEST") != "1" {
		t.Skip("set APE_GPU_ORIENTATION_TEST=1 in a Hyprland session (ffmpeg, grim, hyprctl required)")
	}
	for _, tool := range []string{"ffmpeg", "grim", "hyprctl"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatal(err)
		}
	}
	root := t.TempDir()
	video := filepath.Join(root, "orientation.mp4")
	filter := "color=c=red:s=640x360:r=10,drawbox=x=320:y=0:w=320:h=180:color=lime:t=fill,drawbox=x=0:y=180:w=320:h=180:color=blue:t=fill,drawbox=x=320:y=180:w=320:h=180:color=yellow:t=fill"
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", filter, "-t", "20", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", video)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %s %v", out, err)
	}
	bin := filepath.Join(root, "ape-player")
	if out, err := exec.Command("go", "build", "-tags", "mygo_noinspector", "-o", bin, "../../cmd/ape-player").CombinedOutput(); err != nil {
		t.Fatalf("build: %s %v", out, err)
	}
	for _, backend := range []string{"wayland", "x11"} {
		t.Run(backend, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			state := t.TempDir()
			cmd := exec.CommandContext(ctx, bin, "--play", video)
			cmd.Env = append(os.Environ(), "GDK_BACKEND="+backend, "XDG_CONFIG_HOME="+t.TempDir(), "XDG_STATE_HOME="+state)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
			type window struct {
				PID  int    `json:"pid"`
				At   [2]int `json:"at"`
				Size [2]int `json:"size"`
			}
			var target window
			for {
				b, _ := os.ReadFile(filepath.Join(state, "ape-player", "playback.log"))
				if strings.Contains(string(b), `"event":"mpv.video"`) {
					out, err := exec.CommandContext(ctx, "hyprctl", "clients", "-j").Output()
					if err != nil {
						t.Fatal(err)
					}
					var windows []window
					if err := json.Unmarshal(out, &windows); err != nil {
						t.Fatal(err)
					}
					for _, w := range windows {
						if w.PID == cmd.Process.Pid {
							target = w
						}
					}
					if target.Size[0] > 0 && target.Size[1] > 0 {
						break
					}
				}
				select {
				case <-ctx.Done():
					t.Fatalf("video/window did not become ready; log: %s", b)
				case <-time.After(100 * time.Millisecond):
				}
			}
			// Allow the frame queued by mpv's playback-restart event to reach the compositor.
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(time.Second):
			}
			capture := filepath.Join(root, "orientation-"+backend+".png")
			geometry := fmt.Sprintf("%d,%d %dx%d", target.At[0], target.At[1], target.Size[0], target.Size[1])
			if out, err := exec.CommandContext(ctx, "grim", "-s", "1", "-g", geometry, capture).CombinedOutput(); err != nil {
				t.Fatalf("capture: %s %v", out, err)
			}
			file, err := os.Open(capture)
			if err != nil {
				t.Fatal(err)
			}
			img, err := png.Decode(file)
			file.Close()
			if err != nil {
				t.Fatal(err)
			}
			w, h := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
			scale := math.Min(w/640, h/360)
			samples := []struct {
				name    string
				x, y    float64
				r, g, b bool
			}{
				{"top-left red", -1, -1, true, false, false},
				{"top-right green", 1, -1, false, true, false},
				{"bottom-left blue", -1, 1, false, false, true},
				{"bottom-right yellow", 1, 1, true, true, false},
			}
			for _, s := range samples {
				r, g, b, _ := img.At(int(w/2+s.x*160*scale), int(h/2+s.y*90*scale)).RGBA()
				match := func(v uint32, on bool) bool {
					if on {
						return v > 180*257
					}
					return v < 80*257
				}
				if !match(r, s.r) || !match(g, s.g) || !match(b, s.b) {
					t.Errorf("%s: RGB=(%d,%d,%d); video orientation is wrong", s.name, r/257, g/257, b/257)
				}
			}
			if dir := os.Getenv("APE_GPU_ARTIFACT_DIR"); dir != "" {
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				b, err := os.ReadFile(capture)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, filepath.Base(capture)), b, 0600); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
