<div align="center">

<img src="build/appicon.png" alt="NetFixer Logo" width="96" height="96" />

# NetFixer

**Быстрое и красивое восстановление сети Windows после VPN и сетевых сбоев**

[![Platform](https://img.shields.io/badge/Platform-Windows%2010%20%2F%2011-0078D6?style=for-the-badge&logo=windows)](https://github.com/FindingSelfInDespair/NetFixer/releases/latest)
[![Built with](https://img.shields.io/badge/Go%20%2B%20Wails-v2-00ADD8?style=for-the-badge&logo=go)](https://wails.io/)
[![Release](https://img.shields.io/github/v/release/FindingSelfInDespair/NetFixer?style=for-the-badge&color=10b981)](https://github.com/FindingSelfInDespair/NetFixer/releases/latest)

[⬇️ **Скачать NetFixer.exe (Последняя версия)**](https://github.com/FindingSelfInDespair/NetFixer/releases/latest)

</div>

---

## ✨ О программе

Часто после долгого использования VPN, прокси-клиентов или сбоев роутера в Windows «ломается» интернет: сайты не открываются, остаётся включённым старый локальный прокси или сбивается DNS.

**NetFixer** решает эту проблему в один клик — мгновенно запускается, сам диагностирует причину поломки и мягко восстанавливает сетевые настройки.

### 🎛 Два режима работы
- **Обычный режим** — создан для тех, кто не хочет разбираться в технических деталях. Понятный статус (*«Мешает прокси от VPN»*, *«Сбой DNS»*, *«Интернет работает отлично»*) и одна кнопка **«Починить интернет»**.
- **Профи режим** — тонкая настройка: выбор DNS (`Авто DHCP`, `1.1.1.1 Cloudflare`, `8.8.8.8 Google`), обновление IP-адреса (`/release` и `/renew`), глубокий сброс (`ARP`, `NetBIOS`, `IPv4/IPv6`, `Firewall`) и живой журнал команд.

### 🛠 Что именно чинит программа
- Отключает застрявший системный прокси (`WinINET` + `PAC`-скрипты) и сбрасывает `WinHTTP` прокси.
- Очищает кэш DNS (`ipconfig /flushdns`) и сбрасывает сбитый DNS на автоматический или выбранный.
- Сбрасывает каталог `Winsock` и стек `TCP/IP`.
- Обновляет аренду IP-адреса у роутера.
- Поддерживает тёмную и светлую тему оформления.

---

## 📥 Установка и запуск

1. Скачай готовый **`NetFixer.exe`** со страницы [**Releases**](https://github.com/FindingSelfInDespair/NetFixer/releases/latest).
2. Запусти файл и подтверди запрос прав администратора (UAC).
3. Нажми **«Починить интернет»**.

> **Примечание о Windows SmartScreen:** так как у программы нет платной цифровой подписи, при первом запуске может появиться синее окно защиты Windows. Нажми **«Подробнее» → «Выполнить в любом случае»**.

---

## 🧑‍💻 Сборка из исходников

Требуются [Go](https://go.dev/dl/), [Node.js](https://nodejs.org/) и [Wails CLI v2](https://wails.io/):

```bash
git clone https://github.com/FindingSelfInDespair/NetFixer.git
cd NetFixer
wails build -o NetFixer.exe
<div align="center">
Made with ❤️ by <a href="https://github.com/FindingSelfInDespair">tetraquark</a>
</div>
```