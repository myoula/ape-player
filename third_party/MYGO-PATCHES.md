# MyGO 本地扩展

上游：<https://github.com/egoist/mygo>

基线提交：`4d7114c4522498f991f7ebcba9edc9af0f820c6a`。上游 MIT 许可证保留在 `mygo/LICENSE`。根 module 用 `replace` 固定此副本，构建无需依赖 `mygo-reference`。

## GPU 位图

- `ui.NewGLBitmap(width, height, render, release)` 创建参与正常布局、裁剪、层级和纹理合成的 GPU 图像；`ResizeGL` 保持图像和生产者身份，调整渲染目标。
- `render(contextID, framebufferID, width, height)` 在窗口 GL 线程、上下文当前时调用。FBO 和纹理由框架管理；生产者只绘制到提供的 FBO，并遵守标准 GL 状态约定。
- `release()` 在同一上下文中调用，供生产者销毁自己的 GL 资源；触发于图像离开场景或 renderer 释放。GTK unrealize 前先使上下文 current，并发送已有的 `SurfaceRenew`，确保释放在上下文销毁前完成。
- 回调不可等待 UI，也不可在其中发起网络请求。libmpv 的更新回调只唤醒 UI；命令和事件循环在独立 goroutine 上运行。
- 外部图像在图集上传和 UI 批次之前绘制，之后恢复 FBO、纹理单元、像素解包和相关 GL 状态。普通 UI 仍按既有顺序绘制。
- CPU、Metal、D3D11 使用黑色位图占位；应用首版只启用 Linux GPU 播放，并在 GPU 未初始化时提示错误。不进行逐帧 CPU 回读。

新增 `internal/gpu/gl/external_test.go` 验证画面合成、上层原生 UI、尺寸变化及释放时机。修改的其他文件为 `ui/bitmap.go`、`internal/scene/scene.go`、`internal/gpu/gl/gl.go`、`internal/linux/surface.go`。
