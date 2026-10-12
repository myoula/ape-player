package app

import (
	"ape-player/internal/diagnostics"
	"ape-player/internal/emby"
	"fmt"
	"time"
)

// Diagnose exercises the configured source and actual native GPU backend. It
// accepts ordinary library items, without requiring synthetic subtitle tracks.
func (a *App) Diagnose(itemID string) error {
	result := fmt.Errorf("诊断未完成")
	done := make(chan struct{})
	a.onReady = func() {
		a.activate(a.cfg.Active)
		go func() {
			ticker := time.NewTicker(250 * time.Millisecond)
			defer ticker.Stop()
			deadline := time.Now().Add(60 * time.Second)
			started, finished := false, false
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
				}
				a.update(func() {
					if finished || a.closing {
						return
					}
					if !started && a.canBrowse() && !a.busy {
						started = true
						a.prepare(emby.Item{ID: itemID, Name: "播放诊断", Type: "Movie"}, false, 0, nil, nil)
					}
					if a.player != nil {
						a.player.Command("set", "mute", "yes")
						s := a.state
						if s.Position >= 3 && s.Width > 0 && a.player.FrameCount() >= 20 {
							diagnostics.Log("diagnostic.success", "position", s.Position, "frames", a.player.FrameCount(), "video_codec", s.VideoCodec, "audio_codec", s.AudioCodec, "width", s.Width, "height", s.Height, "hwdec", s.HWDec, "dropped", s.DroppedFrames)
							fmt.Printf("直放成功：%s + %s，%dx%d，hwdec=%s，position=%.2f，frames=%d，dropped=%d\n", s.VideoCodec, s.AudioCodec, s.Width, s.Height, s.HWDec, s.Position, a.player.FrameCount(), s.DroppedFrames)
							result = nil
							finished = true
							a.win.Close()
							return
						}
					}
					if time.Now().After(deadline) || (started && !a.busy && a.player == nil && a.message != "") {
						if a.message != "" {
							result = fmt.Errorf("播放诊断失败：%s", a.message)
						} else {
							result = fmt.Errorf("播放诊断超时：60 秒内未确认视频持续播放（进度 %.2f 秒），详情见播放日志", a.state.Position)
						}
						diagnostics.Log("diagnostic.failed", "error", result.Error())
						finished = true
						a.win.Close()
					}
				})
			}
		}()
	}
	err := a.Run("")
	close(done)
	if err != nil {
		return err
	}
	return result
}
