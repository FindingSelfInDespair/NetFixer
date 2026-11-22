import './style.css';
import {
  IsAdmin,
  RestartAsAdmin,
  DiagnoseNetwork,
  StartFix,
  SetWindowTheme,
  OpenURL,
  RebootNow,
  CancelReboot,
} from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';

const el = (id) => document.getElementById(id);

let currentMode = localStorage.getItem('nf-mode') || 'simple';
let currentTheme = localStorage.getItem('nf-theme') || 'dark';
let selectedDns = 'dhcp';
let isFixing = false;
let lastDiag = null;
const logHistory = [];

const ICONS = {
  ok: `<svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>`,
  warn: `<svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>`,
  err: `<svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><line x1="1" y1="1" x2="23" y2="23"/><path d="M16.72 11.06A10.94 10.94 0 0 1 19 12.55"/><path d="M5 12.55a10.94 10.94 0 0 1 5.17-2.39"/><path d="M10.71 5.05A16 16 0 0 1 22.58 9"/><path d="M1.42 9a15.91 15.91 0 0 1 4.7-2.88"/><path d="M8.53 16.11a6 6 0 0 1 6.95 0"/><line x1="12" y1="20" x2="12.01" y2="20"/></svg>`,
  fixing: `<svg width="38" height="38" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" style="animation: spin 1.1s linear infinite;"><path d="M21 12a9 9 0 1 1-6.219-8.56"/></svg>`,
};

function applyTheme(theme) {
  currentTheme = theme;
  document.documentElement.setAttribute('data-theme', theme === 'light' ? 'light' : 'dark');
  localStorage.setItem('nf-theme', theme);
  SetWindowTheme(theme).catch(() => {});
}

function setMode(mode) {
  currentMode = mode;
  localStorage.setItem('nf-mode', mode);
  el('modeSwitch').setAttribute('data-mode', mode);

  document.querySelectorAll('.mode-btn').forEach((btn) => {
    btn.classList.toggle('active', btn.dataset.target === mode);
  });

  if (mode === 'pro') {
    el('simpleView').classList.add('hidden');
    el('proView').classList.remove('hidden');
    el('modeFootLabel').textContent = 'Профессиональный режим • v2.0';
  } else {
    el('proView').classList.add('hidden');
    el('simpleView').classList.remove('hidden');
    el('modeFootLabel').textContent = 'Обычный режим • v2.0';
  }
}

function setVisualState(state) {
  el('ambientBg').className = `ambient-bg state-${state}`;
  el('heroOrb').className = `hero-orb state-${state}`;
  el('heroIcon').innerHTML = ICONS[state] || ICONS.ok;
}

function appendLog(text, level = 'info') {
  const time = new Date().toLocaleTimeString('ru-RU', { hour12: false });
  const entry = `[${time}] ${text}`;
  logHistory.push({ entry, level });

  const renderInto = (box) => {
    const line = document.createElement('div');
    line.className = `log-line log-${level}`;
    line.textContent = entry;
    box.appendChild(line);
    box.scrollTop = box.scrollHeight;
  };

  renderInto(el('proLogBox'));
  renderInto(el('drawerLogBox'));
}

function clearLogs() {
  logHistory.length = 0;
  el('proLogBox').innerHTML = '';
  el('drawerLogBox').innerHTML = '';
}

function updateSimpleStatus(d) {
  // Обновляем 3 чипа
  const pillNet = el('pillInternet');
  const pillDns = el('pillDns');
  const pillProxy = el('pillProxy');

  if (d.internetOK) {
    pillNet.className = 's-pill ok';
    el('pillInternetVal').textContent = `Есть (${d.pingMS} мс)`;
  } else {
    pillNet.className = 's-pill err';
    el('pillInternetVal').textContent = 'Нет ответа';
  }

  if (d.dnsOK) {
    pillDns.className = 's-pill ok';
    el('pillDnsVal').textContent = 'Открываются';
  } else {
    pillDns.className = 's-pill err';
    el('pillDnsVal').textContent = 'Не грузятся';
  }

  if (d.proxyEnabled) {
    pillProxy.className = 's-pill warn';
    el('pillProxyVal').textContent = 'Обнаружен!';
  } else {
    pillProxy.className = 's-pill ok';
    el('pillProxyVal').textContent = 'Чисто';
  }

  if (isFixing) return;

  // Главный человеческий вердикт
  if (d.proxyEnabled) {
    setVisualState('warn');
    el('heroTitle').textContent = 'Мешает прокси от VPN';
    el('heroSubtitle').textContent = 'В системе остался включённый прокси. Нажмите кнопку ниже, чтобы убрать его и вернуть интернет.';
  } else if (!d.internetOK) {
    setVisualState('err');
    el('heroTitle').textContent = 'Нет подключения к сети';
    el('heroSubtitle').textContent = 'Не удаётся достучаться до интернета. Нажмите «Починить интернет», чтобы сбросить сетевые настройки.';
  } else if (!d.dnsOK) {
    setVisualState('warn');
    el('heroTitle').textContent = 'Сайты не открываются (DNS)';
    el('heroSubtitle').textContent = 'Подключение есть, но адреса сайтов не определяются. Исправим это в один клик.';
  } else {
    setVisualState('ok');
    el('heroTitle').textContent = 'Интернет работает отлично';
    el('heroSubtitle').textContent = `Всё в порядке (${d.adapterName}, пинг ${d.pingMS} мс). Если что-то подтормаживает — можно сделать профилактический сброс.`;
  }
}

function updateProStatus(d) {
  el('proAdapter').textContent = d.adapterName || 'Не найден';
  el('proAdapter').className = `mini-val ${d.adapterName !== 'Не найден' ? 'ok' : 'err'}`;
  el('proIP').textContent = `IP: ${d.localIP}`;

  if (d.internetOK) {
    el('proInternet').textContent = 'Есть связь';
    el('proInternet').className = 'mini-val ok';
    el('proPing').textContent = `Пинг: ${d.pingMS} мс`;
  } else {
    el('proInternet').textContent = 'Нет ответа';
    el('proInternet').className = 'mini-val err';
    el('proPing').textContent = 'Таймаут 1.1.1.1';
  }

  el('proDns').textContent = d.dnsOK ? 'Работает' : 'Не отвечает';
  el('proDns').className = `mini-val ${d.dnsOK ? 'ok' : 'err'}`;

  if (d.proxyEnabled) {
    el('proProxy').textContent = 'Включён';
    el('proProxy').className = 'mini-val warn';
    el('proProxySub').textContent = d.proxyServer;
  } else {
    el('proProxy').textContent = 'Отключён';
    el('proProxy').className = 'mini-val ok';
    el('proProxySub').textContent = 'Чисто';
  }
}

async function runDiagnostics() {
  const refreshBtn = el('refreshBtn');
  refreshBtn.classList.add('spinning');
  refreshBtn.disabled = true;

  try {
    const d = await DiagnoseNetwork();
    lastDiag = d;

    const pill = el('adminPill');
    if (d.isAdmin) {
      pill.className = 'admin-pill is-ok';
      el('adminPillText').textContent = 'Админ';
      pill.title = 'Запущено с правами администратора';
    } else {
      pill.className = 'admin-pill is-warn';
      el('adminPillText').textContent = 'Нужны права';
      pill.title = 'Нажмите, чтобы перезапустить от имени администратора';
    }

    updateSimpleStatus(d);
    updateProStatus(d);
  } catch (err) {
    appendLog(`Ошибка диагностики: ${err}`, 'err');
  } finally {
    refreshBtn.classList.remove('spinning');
    refreshBtn.disabled = false;
  }
}

async function triggerFix(opts) {
  if (isFixing) return;
  isFixing = true;

  clearLogs();
  setVisualState('fixing');
  el('heroTitle').textContent = 'Чиним интернет...';
  el('heroSubtitle').textContent = 'Пожалуйста, подождите пару секунд';

  el('simpleProgressWrap').classList.remove('hidden');
  el('simpleProgressFill').style.width = '0%';
  el('simpleProgressPct').textContent = '0%';
  el('proProgressFill').style.width = '0%';
  el('proProgressPct').textContent = '0%';

  el('simpleFixBtn').disabled = true;
  el('proFixBtn').disabled = true;
  el('simpleFixBtn').classList.remove('is-done');
  el('proFixBtn').classList.remove('is-done');
  el('simpleFixText').textContent = 'Восстанавливаем...';
  el('proFixText').textContent = 'Выполняется...';

  const started = await StartFix(opts);
  if (!started) {
    isFixing = false;
    el('simpleFixBtn').disabled = false;
    el('proFixBtn').disabled = false;
    el('simpleFixText').textContent = 'Починить интернет';
    el('proFixText').textContent = 'Запустить восстановление';
  }
}

// События от Go
EventsOn('fix:log', (ev) => {
  appendLog(ev.text, ev.level);
});

EventsOn('fix:progress', (ev) => {
  el('simpleProgressFill').style.width = `${ev.percent}%`;
  el('simpleProgressPct').textContent = `${ev.percent}%`;
  if (ev.simpleLabel && isFixing) {
    el('heroSubtitle').textContent = ev.simpleLabel;
  }

  el('proProgressFill').style.width = `${ev.percent}%`;
  el('proProgressPct').textContent = `${ev.percent}%`;
  el('proProgressLabel').textContent = ev.label;
});

EventsOn('fix:done', async () => {
  isFixing = false;
  el('simpleFixBtn').disabled = false;
  el('proFixBtn').disabled = false;

  el('simpleFixBtn').classList.add('is-done');
  el('proFixBtn').classList.add('is-done');
  el('simpleFixText').textContent = 'Готово!';
  el('proFixText').textContent = 'Выполнено!';

  await runDiagnostics();

  setTimeout(() => {
    el('simpleFixBtn').classList.remove('is-done');
    el('proFixBtn').classList.remove('is-done');
    el('simpleFixText').textContent = 'Починить интернет';
    el('proFixText').textContent = 'Запустить восстановление';
    el('simpleProgressWrap').classList.add('hidden');
    el('rebootModal').classList.remove('hidden');
  }, 1100);
});

window.addEventListener('DOMContentLoaded', async () => {
  applyTheme(currentTheme);
  setMode(currentMode);

  // Переключатель Обычный / Профи
  document.querySelectorAll('.mode-btn').forEach((btn) => {
    btn.addEventListener('click', () => setMode(btn.dataset.target));
  });

  el('themeBtn').addEventListener('click', () => {
    applyTheme(currentTheme === 'dark' ? 'light' : 'dark');
  });

  el('refreshBtn').addEventListener('click', runDiagnostics);

  el('adminPill').addEventListener('click', async () => {
    if (lastDiag && !lastDiag.isAdmin) {
      try {
        await RestartAsAdmin();
      } catch (e) {
        appendLog(`Не удалось перезапустить с правами администратора: ${e}`, 'err');
      }
    }
  });

  // Выбор DNS в Профи
  document.querySelectorAll('#dnsSelector .dns-chip').forEach((btn) => {
    btn.addEventListener('click', () => {
      document.querySelectorAll('#dnsSelector .dns-chip').forEach((b) => b.classList.remove('active'));
      btn.classList.add('active');
      selectedDns = btn.dataset.dns;
    });
  });

  // Кнопка в Обычном режиме (идеальные безопасные настройки по умолчанию)
  el('simpleFixBtn').addEventListener('click', () => {
    triggerFix({
      dnsMode: 'dhcp',
      resetIP: true,
      advanced: false,
    });
  });

  // Кнопка в Профи режиме
  el('proFixBtn').addEventListener('click', () => {
    triggerFix({
      dnsMode: selectedDns,
      resetIP: el('resetIpCheck').checked,
      advanced: el('advancedCheck').checked,
    });
  });

  // Журнал и Drawer
  const copyLogs = async (btn) => {
    if (logHistory.length === 0) return;
    await navigator.clipboard.writeText(logHistory.map((x) => x.entry).join('\n'));
    const old = btn.textContent;
    btn.textContent = 'Скопировано!';
    setTimeout(() => (btn.textContent = old), 1400);
  };

  el('copyLogBtn').addEventListener('click', () => copyLogs(el('copyLogBtn')));
  el('drawerCopyBtn').addEventListener('click', () => copyLogs(el('drawerCopyBtn')));
  el('clearLogBtn').addEventListener('click', clearLogs);

  el('openDrawerBtn').addEventListener('click', () => el('logDrawer').classList.remove('hidden'));
  el('closeDrawerBtn').addEventListener('click', () => el('logDrawer').classList.add('hidden'));
  el('logDrawer').addEventListener('click', (e) => {
    if (e.target === el('logDrawer')) el('logDrawer').classList.add('hidden');
  });

  // Ссылка на GitHub tetraquark
  el('authorLink').addEventListener('click', () => {
    OpenURL('https://github.com/FindingSelfInDespair');
  });

  // Перезагрузка
  el('laterBtn').addEventListener('click', () => {
    el('rebootModal').classList.add('hidden');
  });

  el('rebootNowBtn').addEventListener('click', async () => {
    try {
      await RebootNow();
      el('rebootText').textContent = 'Компьютер перезагрузится через 10 секунд...';
      el('rebootNowBtn').classList.add('hidden');
      el('laterBtn').classList.add('hidden');
      el('cancelRebootBtn').classList.remove('hidden');
    } catch (e) {
      appendLog(`Ошибка перезагрузки: ${e}`, 'err');
    }
  });

  el('cancelRebootBtn').addEventListener('click', async () => {
    await CancelReboot();
    el('rebootText').textContent = 'Чтобы Windows на 100% применила глубокие сетевые изменения, лучше перезагрузить компьютер.';
    el('rebootNowBtn').classList.remove('hidden');
    el('laterBtn').classList.remove('hidden');
    el('cancelRebootBtn').classList.add('hidden');
    el('rebootModal').classList.add('hidden');
    appendLog('Перезагрузка отменена.', 'warn');
  });

  appendLog('Программа готова. Выполняю диагностику...', 'info');
  await IsAdmin();
  await runDiagnostics();
});