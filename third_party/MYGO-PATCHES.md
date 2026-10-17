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
- GPU 目标尺寸独立存放于 `scene.GLImage.Width/Height`；CPU 占位仅保留一个不透明黑色像素，布局仍使用视频原始尺寸，各后端按归一化 UV 采样。4K 不再额外分配约 31.6 MiB 的 CPU 黑图，`ResizeGL` 也不分配整帧像素。
- 可选 `Bitmap.FitGLToDisplay()` 按场景的实际设备像素缩小 GL 目标，维持源比例并以源尺寸为上限。计算包含裁剪 UV、旋转/缩放；透视变换保守保留原尺寸。同一位图显示多次时取最大需求，只渲染一次。窗口缩放仅重建框架 FBO/纹理，不释放 mpv 上下文；其他后端继续使用单像素回退。Ape Player 已启用。

新增 `internal/gpu/gl/external_test.go` 验证画面合成、上层原生 UI、尺寸变化及释放时机；`ui/gl_bitmap_test.go` 验证单像素占位、缩放后的布局比例和 CPU 回退，并提供 4K 位图分配基准。修改的其他文件为 `ui/bitmap.go`、`internal/scene/scene.go`、`internal/gpu/gl/gl.go`、`internal/linux/surface.go`。

`internal/gpu/gl/external_size_test.go` 覆盖小窗口、高 DPI、全屏、源尺寸上限、强制比例、裁剪、变换和最小尺寸；真实 OpenGL 测试验证目标缩放后的左右颜色采样、生产者生命周期及同图多处显示。

## 外部图像重绘

新增线程安全的 `Window.Repaint()`，用于只有外部 GPU 图像像素发生变化的帧。请求合并后通过 `surface.Conn.Repaint` 进入 UI，复用已有元素和布局；`Update`/`Invalidate`、输入、主题变化及需要重建的动画优先，尺寸或文字系统变化仍触发正常构建。Ape Player 的 mpv 帧通知使用该入口，进度及交互继续走正常状态更新。

修改 `content.go`、`window.go`、`internal/surface/surface.go` 和 `ui/window.go`、`runtime.go`、`input.go`。`TestContentRepaint` 与 `TestExternalRepaintPreservesPendingBuilds` 覆盖仅重绘、连续请求合并、状态更新先后顺序，以及输入、主题、窗口尺寸和文字缓存变化不会丢失。

## 常驻滚动条

新增 `ui.Element.PersistentScrollbars()`，内容溢出时始终绘制滑块与淡色轨道，保留原有拖动和点击轨道翻页行为。未启用的容器仍仅在悬停或拖动时显示。该选项可按容器启用；应用分集横排目前使用下文的隐藏滚动条选项。

修改 `ui/element.go`、`ui/paint.go`，通过 `go generate ./ui` 更新 `ui/api_gen.go`。`TestPersistentScrollbars` 验证非悬停绘制、拖动、默认隐藏行为与内容不溢出时不显示；应用测试现验证隐藏滚动条后仍能拖动内容并滚动至第 1000 集。

## 横向内容拖动

新增 `ui.Element.DragScroll()`，为横向滚动容器启用鼠标左键拖动内容。水平位移超过 6 DIP 且大于垂直位移后接管手势并取消子卡片点击；轻点、右键菜单、滚轮与滚动条保持原行为，输入框、滑块及自带拖动的控件优先处理自己的输入。偏移同步至 `TrackScroll`，虚拟子卡片离开视图不打断拖动；失焦、容器移除或禁用时取消手势。未启用的容器行为不变，不添加惯性滚动。

修改 `ui/element.go`、`ui/context.go`、`ui/runtime.go`、`ui/input.go`，新增 `ui/scroll_drag.go`，生成 `ui/api_gen.go`。`TestDragScroll*` 覆盖点击阈值、边界、右键、手势取消、虚拟列表、滑块与滚轮；应用在首页和分集横排启用该选项。

## 隐藏滚动条

新增 `ui.Element.HideScrollbars()`，按容器关闭滚动条绘制及轨道命中，优先于 PersistentScrollbars；不影响滚轮、触控板、键盘或 DragScroll。首页和分集横排启用，其他容器保持原行为。修改 element/context/runtime/paint/input 并重新生成 api_gen.go；扩展既有滚动条测试，验证悬停隐藏、轨道不再翻页、拖动及滚轮正常。
