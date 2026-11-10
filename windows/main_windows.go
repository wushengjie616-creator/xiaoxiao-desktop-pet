package main

import (
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	xiaoxiao "xiaoxiao-desktop-pet"
)

var (
	user32              = syscall.NewLazyDLL("user32.dll")
	gdi32               = syscall.NewLazyDLL("gdi32.dll")
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	registerClass       = user32.NewProc("RegisterClassExW")
	createWindow        = user32.NewProc("CreateWindowExW")
	defWindowProc       = user32.NewProc("DefWindowProcW")
	destroyWindow       = user32.NewProc("DestroyWindow")
	showWindow          = user32.NewProc("ShowWindow")
	getMessage          = user32.NewProc("GetMessageW")
	translateMessage    = user32.NewProc("TranslateMessage")
	dispatchMessage     = user32.NewProc("DispatchMessageW")
	postMessage         = user32.NewProc("PostMessageW")
	postQuitMessage     = user32.NewProc("PostQuitMessage")
	setTimer            = user32.NewProc("SetTimer")
	killTimer           = user32.NewProc("KillTimer")
	updateLayeredWindow = user32.NewProc("UpdateLayeredWindow")
	getCursorPos        = user32.NewProc("GetCursorPos")
	getAsyncKeyState    = user32.NewProc("GetAsyncKeyState")
	loadCursor          = user32.NewProc("LoadCursorW")
	setCapture          = user32.NewProc("SetCapture")
	releaseCapture      = user32.NewProc("ReleaseCapture")
	createPopupMenu     = user32.NewProc("CreatePopupMenu")
	appendMenu          = user32.NewProc("AppendMenuW")
	trackPopupMenu      = user32.NewProc("TrackPopupMenu")
	destroyMenu         = user32.NewProc("DestroyMenu")
	setForegroundWindow = user32.NewProc("SetForegroundWindow")
	monitorFromWindow   = user32.NewProc("MonitorFromWindow")
	getMonitorInfo      = user32.NewProc("GetMonitorInfoW")
	getSystemMetrics    = user32.NewProc("GetSystemMetrics")
	systemParameters    = user32.NewProc("SystemParametersInfoW")
	dpiContext          = user32.NewProc("SetProcessDpiAwarenessContext")
	getWindowDPI        = user32.NewProc("GetDpiForWindow")
	getLastInput        = user32.NewProc("GetLastInputInfo")
	messageBox          = user32.NewProc("MessageBoxW")
	createDC            = gdi32.NewProc("CreateCompatibleDC")
	createDIB           = gdi32.NewProc("CreateDIBSection")
	selectObject        = gdi32.NewProc("SelectObject")
	deleteObject        = gdi32.NewProc("DeleteObject")
	deleteDC            = gdi32.NewProc("DeleteDC")
	getModuleHandle     = kernel32.NewProc("GetModuleHandleW")
	getTickCount        = kernel32.NewProc("GetTickCount")
	createMutex         = kernel32.NewProc("CreateMutexW")
	closeHandle         = kernel32.NewProc("CloseHandle")
	current             *pet
)

const (
	wmDestroy        = 0x0002
	wmClose          = 0x0010
	wmSettingChange  = 0x001a
	wmMouseActivate  = 0x0021
	wmTimer          = 0x0113
	wmMouseMove      = 0x0200
	wmLButtonDown    = 0x0201
	wmLButtonUp      = 0x0202
	wmRButtonUp      = 0x0205
	wmCaptureChanged = 0x0215
	wmDPIChanged     = 0x02e0

	wsPopup        = 0x80000000
	wsExTopmost    = 0x00000008
	wsExToolWindow = 0x00000080
	wsExLayered    = 0x00080000
	wsExNoActivate = 0x08000000
	autoCommand    = 10
	resetCommand   = 11
	exitCommand    = 12
	poseBase       = 100
	sizeBase       = 200
)

type point struct{ x, y int32 }
type rect struct{ left, top, right, bottom int32 }

type message struct {
	window         uintptr
	id             uint32
	wParam, lParam uintptr
	time           uint32
	cursor         point
	private        uint32
}

type windowClass struct {
	size, style                        uint32
	proc                               uintptr
	classExtra, windowExtra            int32
	instance, icon, cursor, background uintptr
	menu, name                         *uint16
	smallIcon                          uintptr
}

type monitorInfo struct {
	size          uint32
	monitor, work rect
	flags         uint32
}

type bitmapHeader struct {
	size                        uint32
	width, height               int32
	planes, bits                uint16
	compression, imageSize      uint32
	xPixels, yPixels            int32
	colorsUsed, colorsImportant uint32
}

type surface struct {
	dc, bitmap, previous uintptr
	pixels               []byte
	w, h                 int
}

func (s *surface) close() {
	if s.dc != 0 {
		selectObject.Call(s.dc, s.previous)
		deleteObject.Call(s.bitmap)
		deleteDC.Call(s.dc)
	}
	*s = surface{}
}

func (s *surface) resize(w, h int) error {
	if s.w == w && s.h == h {
		return nil
	}
	s.close()
	dc, _, err := createDC.Call(0)
	if dc == 0 {
		return err
	}
	header := bitmapHeader{size: 40, width: int32(w), height: -int32(h), planes: 1, bits: 32}
	var pixels *byte
	bitmap, _, err := createDIB.Call(dc, uintptr(unsafe.Pointer(&header)), 0,
		uintptr(unsafe.Pointer(&pixels)), 0, 0)
	if bitmap == 0 || pixels == nil {
		if bitmap != 0 {
			deleteObject.Call(bitmap)
		}
		deleteDC.Call(dc)
		return err
	}
	previous, _, err := selectObject.Call(dc, bitmap)
	if previous == 0 || previous == ^uintptr(0) {
		deleteObject.Call(bitmap)
		deleteDC.Call(dc)
		return err
	}
	*s = surface{dc, bitmap, previous, unsafe.Slice(pixels, w*h*4), w, h}
	return nil
}

type pet struct {
	window                       uintptr
	clip                         *animation
	sit                          *image.RGBA
	pose, size, nextActive       int
	automatic, reduced, menuOpen bool
	started, lastTick, nextAuto  time.Time
	position                     point
	walk                         patrol
	surface                      surface
	holding, dragged             bool
	mouseStart, windowStart      point
	lastFrame                    *image.RGBA
	lastPosition                 point
	lastMirror                   bool
}

func wide(text string) *uint16 {
	value, _ := syscall.UTF16PtrFromString(text)
	return value
}

func cursor() (p point) {
	getCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return
}

func primaryArea() (area rect) {
	ok, _, _ := systemParameters.Call(0x0030, 0, uintptr(unsafe.Pointer(&area)), 0)
	if ok == 0 {
		w, _, _ := getSystemMetrics.Call(0)
		h, _, _ := getSystemMetrics.Call(1)
		area = rect{right: int32(w), bottom: int32(h)}
	}
	return
}

func workArea(window uintptr) rect {
	monitor, _, _ := monitorFromWindow.Call(window, 2)
	info := monitorInfo{size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if monitor != 0 {
		ok, _, _ := getMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info)))
		if ok != 0 {
			return info.work
		}
	}
	return primaryArea()
}

func loadPose(index int) (*animation, error) {
	data, err := xiaoxiao.Animation(poses[index].key)
	if err != nil {
		return nil, err
	}
	return decodeAPNG(data)
}

func (p *pet) updateAccessibility() {
	var enabled int32 = 1
	systemParameters.Call(0x1042, 0, uintptr(unsafe.Pointer(&enabled)), 0)
	p.reduced = enabled == 0
}

func (p *pet) dimensions() (int, int, float64) {
	dpi := uintptr(96)
	if getWindowDPI.Find() == nil {
		if value, _, _ := getWindowDPI.Call(p.window); value != 0 {
			dpi = value
		}
	}
	box := p.clip.frames[0].Bounds()
	scale := float64(p.size) * float64(dpi) / (96 * float64(max(box.Dx(), box.Dy())))
	return max(1, int(math.Round(float64(box.Dx())*scale))),
		max(1, int(math.Round(float64(box.Dy())*scale))), scale
}

func (p *pet) changePose(index int, now time.Time) error {
	if p.clip != nil && index == p.pose {
		return nil
	}
	clip, err := loadPose(index)
	if err != nil {
		return fmt.Errorf("无法打开「%s」动画：%w", poses[index].title, err)
	}
	x, y := p.position.x, p.position.y
	if p.clip != nil {
		w, h, _ := p.dimensions()
		x += int32(w / 2)
		y += int32(h)
	}
	p.clip, p.pose, p.started = clip, index, now
	w, h, _ := p.dimensions()
	if !p.lastTick.IsZero() {
		area := workArea(p.window)
		p.position = point{min(max(x-int32(w/2), area.left), max(area.left, area.right-int32(w))),
			min(max(y-int32(h), area.top), max(area.top, area.bottom-int32(h)))}
	}
	p.walk = patrol{x: float64(p.position.x), direction: -1}
	p.lastFrame = nil
	return nil
}

func (p *pet) resetPosition() {
	area := workArea(p.window)
	w, h, _ := p.dimensions()
	p.position = point{max(area.left, area.right-int32(w)-20), max(area.top, area.bottom-int32(h)-20)}
	p.walk.x = float64(p.position.x)
}

func (p *pet) present(now time.Time) error {
	w, h, _ := p.dimensions()
	resize := p.surface.w != w || p.surface.h != h
	if err := p.surface.resize(w, h); err != nil {
		return err
	}

	frame := p.clip.frames[0]
	mirror := poses[p.pose].stride > 0 && p.walk.direction > 0
	if !p.walk.restUntil.IsZero() {
		frame, mirror = p.sit, false
	} else if !p.reduced {
		frame = p.clip.frame(now.Sub(p.started))
	}
	repaint := resize || p.lastFrame != frame || mirror != p.lastMirror
	if !repaint && p.lastPosition == p.position {
		return nil
	}
	if repaint {
		renderBGRA(p.surface.pixels, w, h, frame, mirror)
	}
	size := point{int32(w), int32(h)}
	src := point{}
	blend := [4]byte{0, 0, 255, 1}
	ok, _, err := updateLayeredWindow.Call(p.window, 0,
		uintptr(unsafe.Pointer(&p.position)), uintptr(unsafe.Pointer(&size)),
		p.surface.dc, uintptr(unsafe.Pointer(&src)), 0,
		uintptr(unsafe.Pointer(&blend)), 2)
	if ok == 0 {
		return err
	}
	p.lastFrame, p.lastPosition, p.lastMirror = frame, p.position, mirror
	return nil
}

func idleTime() time.Duration {
	info := [2]uint32{8, 0}
	ok, _, _ := getLastInput.Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		return 0
	}
	tick, _, _ := getTickCount.Call()
	return time.Duration(uint32(tick)-info[1]) * time.Millisecond
}

func (p *pet) autoPose(now time.Time) error {
	idle := idleTime()
	switch {
	case idle >= 5*time.Minute:
		return p.changePose(sleeping, now)
	case idle >= 90*time.Second:
		return p.changePose(resting, now)
	case !now.Before(p.nextAuto):
		index := active[p.nextActive]
		p.nextActive = (p.nextActive + 1) % len(active)
		p.nextAuto = now.Add(16 * time.Second)
		return p.changePose(index, now)
	default:
		return p.changePose(active[(p.nextActive+len(active)-1)%len(active)], now)
	}
}

func (p *pet) tick(now time.Time) error {
	elapsed := min(now.Sub(p.lastTick).Seconds(), 0.15)
	p.lastTick = now
	if p.holding {
		p.dragToCursor()
		buttonKey := uintptr(1)
		if swapped, _, _ := getSystemMetrics.Call(23); swapped != 0 {
			buttonKey = 2
		}
		button, _, _ := getAsyncKeyState.Call(buttonKey)
		if button&0x8000 == 0 {
			if err := p.finishDrag(now); err != nil {
				return err
			}
		}
	}
	if p.automatic && !p.holding && !p.menuOpen {
		if err := p.autoPose(now); err != nil {
			return err
		}
	}
	if stride := poses[p.pose].stride; stride > 0 && !p.reduced && !p.holding && !p.menuOpen {
		area := workArea(p.window)
		w, _, scale := p.dimensions()
		if p.walk.advance(now, elapsed, stride*scale/p.clip.duration.Seconds(),
			float64(area.left+12), float64(area.right-int32(w)-12)) {
			p.started = now
		}
		p.position.x = int32(math.Round(p.walk.x))
	}
	return p.present(now)
}

func menuItem(menu uintptr, flags uint32, id uintptr, title string) {
	appendMenu.Call(menu, uintptr(flags), id, uintptr(unsafe.Pointer(wide(title))))
}

func (p *pet) openMenu() error {
	menu, _, err := createPopupMenu.Call()
	if menu == 0 {
		return err
	}
	defer destroyMenu.Call(menu)
	check := func(value bool) uint32 {
		if value {
			return 8
		}
		return 0
	}
	menuItem(menu, check(p.automatic), autoCommand, "自动切换姿态")
	for i, item := range poses {
		menuItem(menu, check(!p.automatic && p.pose == i), uintptr(poseBase+i), item.title)
	}
	menuItem(menu, 0x800, 0, "")
	sizeMenu, _, _ := createPopupMenu.Call()
	if sizeMenu != 0 {
		for i, size := range sizes {
			menuItem(sizeMenu, check(p.size == size), uintptr(sizeBase+i), fmt.Sprintf("%d 点", size))
		}
		menuItem(menu, 0x10, sizeMenu, "大小")
	}
	menuItem(menu, 0, resetCommand, "移回右下角")
	menuItem(menu, 0, exitCommand, "退出小小")

	at := cursor()
	p.menuOpen = true
	setForegroundWindow.Call(p.window)
	command, _, _ := trackPopupMenu.Call(menu, 0x0102, uintptr(at.x), uintptr(at.y), 0, p.window, 0)
	postMessage.Call(p.window, 0, 0, 0)
	p.menuOpen = false
	now := time.Now()
	switch {
	case command == exitCommand:
		destroyWindow.Call(p.window)
		return nil
	case command == resetCommand:
		p.resetPosition()
	case command == autoCommand:
		p.automatic, p.nextAuto = true, time.Time{}
		if err := p.autoPose(now); err != nil {
			return err
		}
	case command >= poseBase && command < uintptr(poseBase+len(poses)):
		p.automatic = false
		if err := p.changePose(int(command)-poseBase, now); err != nil {
			return err
		}
	case command >= sizeBase && command < uintptr(sizeBase+len(sizes)):
		p.size = sizes[int(command)-sizeBase]
		saveSize(p.size)
		w, h, _ := p.dimensions()
		area := workArea(p.window)
		p.position.x = min(max(p.position.x, area.left), max(area.left, area.right-int32(w)))
		p.position.y = min(max(p.position.y, area.top), max(area.top, area.bottom-int32(h)))
		p.walk.x = float64(p.position.x)
	}
	return p.present(now)
}

func preferencePath() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "Xiaoxiao", "size.txt")
}

func loadSize() int {
	data, _ := os.ReadFile(preferencePath())
	value, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	for _, size := range sizes {
		if value == size {
			return size
		}
	}
	return 240
}

func saveSize(size int) {
	path := preferencePath()
	if path != "" && os.MkdirAll(filepath.Dir(path), 0755) == nil {
		_ = os.WriteFile(path, []byte(strconv.Itoa(size)), 0644)
	}
}

func showError(err error) {
	messageBox.Call(0, uintptr(unsafe.Pointer(wide(err.Error()))),
		uintptr(unsafe.Pointer(wide("小小"))), 0x10)
}

func (p *pet) dragToCursor() {
	pos := cursor()
	dx, dy := pos.x-p.mouseStart.x, pos.y-p.mouseStart.y
	if p.dragged || math.Hypot(float64(dx), float64(dy)) >= 4 {
		p.dragged = true
		p.position = point{p.windowStart.x + dx, p.windowStart.y + dy}
		p.walk.x = float64(p.position.x)
	}
}

func (p *pet) finishDrag(now time.Time) error {
	if !p.holding {
		return nil
	}
	p.holding = false
	releaseCapture.Call()
	if !p.dragged {
		p.automatic = false
		return p.changePose((p.pose+1)%len(poses), now)
	}
	// Keep at least the cat itself reachable after a drag to the screen edge.
	w, h, _ := p.dimensions()
	area := workArea(p.window)
	p.position.x = min(max(p.position.x, area.left), max(area.left, area.right-int32(w)))
	p.position.y = min(max(p.position.y, area.top), max(area.top, area.bottom-int32(h)))
	p.walk.x = float64(p.position.x)
	return nil
}

func windowProc(window uintptr, id uint32, wParam, lParam uintptr) uintptr {
	p := current
	if p != nil {
		var err error
		switch id {
		case wmMouseActivate:
			return 3
		case wmLButtonDown:
			p.holding, p.dragged = true, false
			p.mouseStart, p.windowStart = cursor(), p.position
			setCapture.Call(window)
			return 0
		case wmMouseMove:
			if p.holding {
				p.dragToCursor()
			}
			return 0
		case wmLButtonUp:
			err = p.finishDrag(time.Now())
		case wmCaptureChanged:
			p.holding = false
			return 0
		case wmRButtonUp:
			err = p.openMenu()
		case wmSettingChange:
			p.updateAccessibility()
			return 0
		case wmDPIChanged:
			p.lastFrame = nil
			return 0
		case wmTimer:
			err = p.tick(time.Now())
		case wmClose:
			destroyWindow.Call(window)
			return 0
		case wmDestroy:
			killTimer.Call(window, 1)
			p.surface.close()
			postQuitMessage.Call(0)
			return 0
		default:
			result, _, _ := defWindowProc.Call(window, uintptr(id), wParam, lParam)
			return result
		}
		if err != nil {
			killTimer.Call(window, 1)
			showError(err)
			destroyWindow.Call(window)
		}
		return 0
	}
	result, _, _ := defWindowProc.Call(window, uintptr(id), wParam, lParam)
	return result
}

func run() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	mutex, _, mutexErr := createMutex.Call(0, 0, uintptr(unsafe.Pointer(wide("Local\\XiaoxiaoDesktopPet"))))
	if mutex == 0 {
		return mutexErr
	}
	defer closeHandle.Call(mutex)
	if mutexErr == syscall.Errno(183) {
		return nil
	}
	if dpiContext.Find() == nil {
		dpiContext.Call(^uintptr(3)) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2
	}
	sit, err := loadPose(sitting)
	if err != nil {
		return err
	}
	p := &pet{sit: sit.frames[0], size: loadSize(), automatic: true}
	p.updateAccessibility()
	current = p
	instance, _, _ := getModuleHandle.Call(0)
	name := wide("XiaoxiaoDesktopPet")
	arrow, _, _ := loadCursor.Call(0, 32512)
	class := windowClass{cursor: arrow, size: uint32(unsafe.Sizeof(windowClass{})), name: name, instance: instance,
		proc: syscall.NewCallback(windowProc)}
	if result, _, err := registerClass.Call(uintptr(unsafe.Pointer(&class))); result == 0 {
		return err
	}
	flags := wsExLayered | wsExTopmost | wsExToolWindow | wsExNoActivate
	window, _, err := createWindow.Call(uintptr(flags), uintptr(unsafe.Pointer(name)),
		uintptr(unsafe.Pointer(wide("小小"))), wsPopup, 0, 0, 1, 1, 0, 0, instance, 0)
	runtime.KeepAlive(name)
	if window == 0 {
		return err
	}
	p.window = window
	now := time.Now()
	if err := p.autoPose(now); err != nil {
		destroyWindow.Call(window)
		return err
	}
	p.resetPosition()
	p.lastTick = now
	if err := p.present(now); err != nil {
		destroyWindow.Call(window)
		return err
	}
	showWindow.Call(window, 4) // SW_SHOWNOACTIVATE
	if timer, _, err := setTimer.Call(window, 1, 16, 0); timer == 0 {
		destroyWindow.Call(window)
		return err
	}
	var msg message
	for {
		result, _, err := getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		switch int32(result) {
		case -1:
			return err
		case 0:
			return nil
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		dispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func main() {
	if err := run(); err != nil {
		showError(err)
	}
}
