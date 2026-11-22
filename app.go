package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/text/encoding/charmap"
)

type App struct {
	ctx       context.Context
	mu        sync.Mutex
	isRunning bool
}

type DiagResult struct {
	IsAdmin      bool   `json:"isAdmin"`
	AdapterName  string `json:"adapterName"`
	LocalIP      string `json:"localIP"`
	InternetOK   bool   `json:"internetOK"`
	PingMS       int64  `json:"pingMS"`
	DnsOK        bool   `json:"dnsOK"`
	ProxyEnabled bool   `json:"proxyEnabled"`
	ProxyServer  string `json:"proxyServer"`
}

type FixOptions struct {
	DnsMode  string `json:"dnsMode"`  // "dhcp", "cloudflare", "google", "keep"
	ResetIP  bool   `json:"resetIP"`  // ipconfig /release & /renew
	Advanced bool   `json:"advanced"` // arp, nbtstat, ipv4/ipv6 reset, advfirewall
}

type LogEvent struct {
	Text  string `json:"text"`
	Level string `json:"level"` // "info", "cmd", "ok", "warn", "err"
}

type ProgressEvent struct {
	Percent     int    `json:"percent"`
	Label       string `json:"label"`
	SimpleLabel string `json:"simpleLabel"`
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// IsAdmin проверяет, запущена ли программа с правами администратора
func (a *App) IsAdmin() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

// RestartAsAdmin перезапускает приложение с запросом UAC
func (a *App) RestartAsAdmin() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cwd := filepath.Dir(exe)
	verbPtr, _ := windows.UTF16PtrFromString("runas")
	exePtr, _ := windows.UTF16PtrFromString(exe)
	cwdPtr, _ := windows.UTF16PtrFromString(cwd)

	err = windows.ShellExecute(0, verbPtr, exePtr, nil, cwdPtr, windows.SW_SHOWNORMAL)
	if err != nil {
		return err
	}
	runtime.Quit(a.ctx)
	return nil
}

// SetWindowTheme переключает тему заголовка окна Windows
func (a *App) SetWindowTheme(theme string) {
	if theme == "light" {
		runtime.WindowSetLightTheme(a.ctx)
	} else {
		runtime.WindowSetDarkTheme(a.ctx)
	}
}

// OpenURL открывает ссылку в браузере
func (a *App) OpenURL(url string) {
	runtime.BrowserOpenURL(a.ctx, url)
}

// DiagnoseNetwork проводит быструю диагностику состояния сети
func (a *App) DiagnoseNetwork() DiagResult {
	res := DiagResult{
		IsAdmin:     a.IsAdmin(),
		AdapterName: "Не найден",
		LocalIP:     "—",
	}

	adapter, ip := detectActiveInterface()
	if adapter != "" {
		res.AdapterName = adapter
		res.LocalIP = ip
	}

	start := time.Now()
	conn, err := net.DialTimeout("tcp", "1.1.1.1:443", 1600*time.Millisecond)
	if err != nil {
		start = time.Now()
		conn, err = net.DialTimeout("tcp", "8.8.8.8:53", 1600*time.Millisecond)
	}
	if err == nil {
		res.InternetOK = true
		res.PingMS = time.Since(start).Milliseconds()
		_ = conn.Close()
	}

	dnsCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := net.DefaultResolver.LookupHost(dnsCtx, "google.com"); err == nil {
		res.DnsOK = true
	} else if _, err := net.DefaultResolver.LookupHost(dnsCtx, "cloudflare.com"); err == nil {
		res.DnsOK = true
	}

	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err == nil {
		defer k.Close()
		if enabled, _, err := k.GetIntegerValue("ProxyEnable"); err == nil && enabled == 1 {
			res.ProxyEnabled = true
			if srv, _, err := k.GetStringValue("ProxyServer"); err == nil && srv != "" {
				res.ProxyServer = srv
			} else {
				res.ProxyServer = "Включён"
			}
		} else if pac, _, err := k.GetStringValue("AutoConfigURL"); err == nil && pac != "" {
			res.ProxyEnabled = true
			res.ProxyServer = "PAC: " + pac
		}
	}

	return res
}

// StartFix запускает процесс восстановления сети в фоне
func (a *App) StartFix(opts FixOptions) bool {
	a.mu.Lock()
	if a.isRunning {
		a.mu.Unlock()
		return false
	}
	a.isRunning = true
	a.mu.Unlock()

	go func() {
		defer func() {
			a.mu.Lock()
			a.isRunning = false
			a.mu.Unlock()
		}()

		a.emitLog("Запуск процесса восстановления сети...", "info")
		if !a.IsAdmin() {
			a.emitLog("⚠ Программа запущена без прав администратора — часть системных команд может быть отклонена.", "warn")
		}

		type step struct {
			title       string
			simpleTitle string
			fn          func() error
		}

		var steps []step

		steps = append(steps, step{
			title:       "Сброс настроек системного прокси (WinINET)",
			simpleTitle: "Убираем старый прокси от VPN...",
			fn:          a.resetProxyRegistry,
		})

		steps = append(steps, step{
			title:       "Сброс WinHTTP прокси",
			simpleTitle: "Очищаем системные настройки прокси...",
			fn: func() error {
				return a.execCmd(10*time.Second, "netsh", "winhttp", "reset", "proxy")
			},
		})

		steps = append(steps, step{
			title:       "Очистка кэша DNS",
			simpleTitle: "Очищаем кэш адресов сайтов...",
			fn: func() error {
				return a.execCmd(10*time.Second, "ipconfig", "/flushdns")
			},
		})

		steps = append(steps, step{
			title:       "Сброс каталога Winsock",
			simpleTitle: "Восстанавливаем сетевые сокеты Windows...",
			fn: func() error {
				return a.execCmd(15*time.Second, "netsh", "winsock", "reset")
			},
		})

		ipLogPath := filepath.Join(os.TempDir(), "netfixer_ipreset.log")
		steps = append(steps, step{
			title:       "Сброс стека TCP/IP",
			simpleTitle: "Сбрасываем параметры сетевого протокола...",
			fn: func() error {
				return a.execCmd(15*time.Second, "netsh", "int", "ip", "reset", ipLogPath)
			},
		})

		if opts.DnsMode != "" && opts.DnsMode != "keep" {
			steps = append(steps, step{
				title:       fmt.Sprintf("Настройка DNS (%s)", opts.DnsMode),
				simpleTitle: "Настраиваем правильный DNS...",
				fn: func() error {
					return a.applyDNS(opts.DnsMode)
				},
			})
		}

		if opts.ResetIP {
			steps = append(steps, step{
				title:       "Освобождение текущего IP (ipconfig /release)",
				simpleTitle: "Переподключаемся к роутеру...",
				fn: func() error {
					return a.execCmd(20*time.Second, "ipconfig", "/release")
				},
			})
			steps = append(steps, step{
				title:       "Получение нового IP (ipconfig /renew)",
				simpleTitle: "Получаем новый IP-адрес...",
				fn: func() error {
					return a.execCmd(25*time.Second, "ipconfig", "/renew")
				},
			})
		}

		if opts.Advanced {
			steps = append(steps,
				step{
					title:       "Очистка таблицы ARP",
					simpleTitle: "Очищаем таблицу маршрутов...",
					fn: func() error {
						return a.execCmd(10*time.Second, "arp", "-d", "*")
					},
				},
				step{
					title:       "Перезагрузка кэша NetBIOS (nbtstat -R)",
					simpleTitle: "Обновляем кэш локальной сети...",
					fn: func() error {
						return a.execCmd(10*time.Second, "nbtstat", "-R")
					},
				},
				step{
					title:       "Отправка обновления NetBIOS (nbtstat -RR)",
					simpleTitle: "Обновляем имена в локальной сети...",
					fn: func() error {
						return a.execCmd(10*time.Second, "nbtstat", "-RR")
					},
				},
				step{
					title:       "Сброс стека IPv4",
					simpleTitle: "Перезапускаем протокол IPv4...",
					fn: func() error {
						return a.execCmd(15*time.Second, "netsh", "int", "ipv4", "reset")
					},
				},
				step{
					title:       "Сброс стека IPv6",
					simpleTitle: "Перезапускаем протокол IPv6...",
					fn: func() error {
						return a.execCmd(15*time.Second, "netsh", "int", "ipv6", "reset")
					},
				},
				step{
					title:       "Сброс правил Брандмауэра Windows",
					simpleTitle: "Сбрасываем правила брандмауэра...",
					fn: func() error {
						return a.execCmd(15*time.Second, "netsh", "advfirewall", "reset")
					},
				},
			)
		}

		total := len(steps)
		hasWarnings := false

		for i, s := range steps {
			pct := int(float64(i) / float64(total) * 100)
			a.emitProgress(pct, fmt.Sprintf("Шаг %d из %d: %s", i+1, total, s.title), s.simpleTitle)
			a.emitLog(fmt.Sprintf("▶ %s", s.title), "cmd")

			if err := s.fn(); err != nil {
				hasWarnings = true
				a.emitLog(fmt.Sprintf("[!] Замечание: %v", err), "warn")
			} else {
				a.emitLog("[✓] Успешно выполнено", "ok")
			}
			time.Sleep(140 * time.Millisecond)
		}

		a.emitProgress(100, "Восстановление завершено!", "Проверяем итоговое соединение...")
		if hasWarnings {
			a.emitLog("Готово! Часть шагов завершилась с замечаниями — проверь журнал и статус.", "warn")
		} else {
			a.emitLog("✓ Все сетевые компоненты успешно восстановлены!", "ok")
		}

		runtime.EventsEmit(a.ctx, "fix:done", !hasWarnings)
	}()

	return true
}

// RebootNow запускает перезагрузку ПК через 10 секунд
func (a *App) RebootNow() error {
	return a.execCmd(5*time.Second, "shutdown", "/r", "/t", "10", "/c", "Перезагрузка для применения сетевых настроек")
}

// CancelReboot отменяет запланированную перезагрузку
func (a *App) CancelReboot() error {
	return a.execCmd(5*time.Second, "shutdown", "/a")
}

func (a *App) resetProxyRegistry() error {
	k, err := registry.OpenKey(
		registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		registry.QUERY_VALUE|registry.SET_VALUE,
	)
	if err != nil {
		return err
	}
	defer k.Close()

	if err := k.SetDWordValue("ProxyEnable", 0); err != nil {
		return err
	}

	for _, valName := range []string{"ProxyServer", "ProxyOverride", "AutoConfigURL"} {
		err := k.DeleteValue(valName)
		if err != nil && !errors.Is(err, registry.ErrNotExist) {
			return fmt.Errorf("не удалось удалить %s: %w", valName, err)
		}
	}
	return nil
}

func (a *App) applyDNS(mode string) error {
	adapters := getCandidateAdapters()
	if len(adapters) == 0 {
		return fmt.Errorf("не найдены активные сетевые адаптеры")
	}

	applied := 0
	for _, name := range adapters {
		var err error
		switch mode {
		case "dhcp":
			err = a.execCmd(10*time.Second, "netsh", "interface", "ip", "set", "dns", fmt.Sprintf("name=%s", name), "source=dhcp")
			if err != nil {
				low := strings.ToLower(err.Error())
				if strings.Contains(low, "уже включен") || strings.Contains(low, "already enabled") {
					err = nil
				}
			}
		case "cloudflare":
			err = a.execCmd(10*time.Second, "netsh", "interface", "ip", "set", "dns", fmt.Sprintf("name=%s", name), "source=static", "address=1.1.1.1")
			if err == nil {
				_ = a.execCmd(10*time.Second, "netsh", "interface", "ip", "add", "dns", fmt.Sprintf("name=%s", name), "address=1.0.0.1", "index=2")
			}
		case "google":
			err = a.execCmd(10*time.Second, "netsh", "interface", "ip", "set", "dns", fmt.Sprintf("name=%s", name), "source=static", "address=8.8.8.8")
			if err == nil {
				_ = a.execCmd(10*time.Second, "netsh", "interface", "ip", "add", "dns", fmt.Sprintf("name=%s", name), "address=8.8.4.4", "index=2")
			}
		}

		if err == nil {
			applied++
			a.emitLog(fmt.Sprintf("   • DNS обновлён на адаптере «%s»", name), "info")
		}
	}

	if applied == 0 {
		return fmt.Errorf("не удалось обновить DNS ни на одном адаптере")
	}
	return nil
}

func (a *App) execCmd(timeout time.Duration, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}

	out, err := cmd.CombinedOutput()
	decoded := decodeOEM(out)

	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("таймаут команды '%s' (%v)", name, timeout)
	}
	if err != nil {
		if decoded != "" {
			return fmt.Errorf("%v: %s", err, decoded)
		}
		return err
	}
	return nil
}

func decodeOEM(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	if utf8.Valid(raw) {
		return strings.TrimSpace(string(raw))
	}
	decoded, err := charmap.CodePage866.NewDecoder().Bytes(raw)
	if err != nil {
		return strings.TrimSpace(string(raw))
	}
	return strings.TrimSpace(string(decoded))
}

func detectActiveInterface() (string, string) {
	conn, err := net.DialTimeout("udp4", "1.1.1.1:80", 400*time.Millisecond)
	if err == nil {
		localIP := conn.LocalAddr().(*net.UDPAddr).IP
		_ = conn.Close()

		ifaces, err := net.Interfaces()
		if err == nil {
			for _, iface := range ifaces {
				addrs, _ := iface.Addrs()
				for _, addr := range addrs {
					if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP.Equal(localIP) {
						return iface.Name, localIP.String()
					}
				}
			}
		}
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return "", ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if isVirtualAdapter(iface.Name) {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP.To4() != nil {
				return iface.Name, ipNet.IP.String()
			}
		}
		return iface.Name, "Без IPv4"
	}
	return "", ""
}

func getCandidateAdapters() []string {
	var names []string
	active, _ := detectActiveInterface()
	if active != "" {
		names = append(names, active)
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return names
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if isVirtualAdapter(iface.Name) || iface.Name == active {
			continue
		}
		names = append(names, iface.Name)
	}
	return names
}

func isVirtualAdapter(name string) bool {
	lower := strings.ToLower(name)
	skip := []string{"loopback", "vethernet", "vmware", "virtualbox", "wsl", "hyper-v", "tun", "tap", "wireguard"}
	for _, s := range skip {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

func (a *App) emitLog(text, level string) {
	runtime.EventsEmit(a.ctx, "fix:log", LogEvent{
		Text:  text,
		Level: level,
	})
}

func (a *App) emitProgress(percent int, label, simpleLabel string) {
	runtime.EventsEmit(a.ctx, "fix:progress", ProgressEvent{
		Percent:     percent,
		Label:       label,
		SimpleLabel: simpleLabel,
	})
}
