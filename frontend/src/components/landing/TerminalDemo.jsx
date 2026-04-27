import { useEffect, useRef, useState } from 'react';
import { motion } from 'framer-motion';

// One continuous session, ~2-3 minutes, ends on a calm summary.
// Each step can have { en, ru } text — commands/paths/IPs/services stay untranslated.
const session = [
  { t: 'cmd', text: '$ curl -sSL get.goronin.ru/install.sh | bash -s vt_9a2f...' },
  {
    t: 'log',
    en: '→ Detecting OS: Ubuntu 22.04 LTS',
    ru: '→ Определяю ОС: Ubuntu 22.04 LTS',
  },
  {
    t: 'log',
    en: '→ Fetching agent v0.4.1 (linux/amd64)',
    ru: '→ Скачиваю агент v0.4.1 (linux/amd64)',
  },
  {
    t: 'log',
    en: '→ Verifying SHA-256 signature... ok',
    ru: '→ Проверяю SHA-256 подпись... ок',
  },
  {
    t: 'ok',
    en: '✓ Agent installed as systemd service',
    ru: '✓ Агент установлен как systemd-сервис',
  },
  {
    t: 'ok',
    en: '✓ Traps armed on :22, :80, :443, :3306, :5432, :6379',
    ru: '✓ Ловушки активны на :22, :80, :443, :3306, :5432, :6379',
  },
  {
    t: 'ok',
    en: '✓ Paired with account kitay@bitcoff.io',
    ru: '✓ Привязан к аккаунту kitay@bitcoff.io',
  },
  {
    t: 'log',
    en: '→ Telegram bot connected: @goronin_alert_bot',
    ru: '→ Telegram-бот подключён: @goronin_alert_bot',
  },
  {
    t: 'log',
    en: '→ Listening for intrusions...',
    ru: '→ Слушаю вторжения...',
  },
  { t: 'pause', ms: 1800 },

  {
    t: 'evt',
    en: '⚠ 12:03:17  SSH probe from 185.220.101.42 (Tor exit)',
    ru: '⚠ 12:03:17  SSH-зондирование с 185.220.101.42 (Tor exit)',
  },
  {
    t: 'log',
    en: '→ AI: reconnaissance, low confidence',
    ru: '→ AI: разведка, низкая уверенность',
  },
  { t: 'tg', en: '📩 Telegram alert sent', ru: '📩 Алерт отправлен в Telegram' },
  { t: 'pause', ms: 1200 },

  { t: 'evt', text: '⚠ 12:04:02  HTTP GET /.env from 45.142.214.9' },
  { t: 'evt', text: '⚠ 12:04:02  HTTP GET /wp-admin from 45.142.214.9' },
  { t: 'evt', text: '⚠ 12:04:03  HTTP GET /.git/config from 45.142.214.9' },
  {
    t: 'log',
    en: '→ AI: automated web scanner, secrets hunting',
    ru: '→ AI: автосканер, охота за секретами',
  },
  { t: 'pause', ms: 1000 },

  { t: 'evt', text: '⚠ 12:06:44  MySQL auth: root / 123456' },
  { t: 'evt', text: '⚠ 12:06:45  MySQL auth: root / password' },
  { t: 'evt', text: '⚠ 12:06:45  MySQL auth: admin / admin123' },
  {
    t: 'log',
    en: '→ AI: credential stuffing, common wordlist',
    ru: '→ AI: подбор паролей, типовой словарь',
  },
  { t: 'tg', en: '📩 Telegram alert sent', ru: '📩 Алерт отправлен в Telegram' },
  { t: 'pause', ms: 1400 },

  { t: 'evt', text: '⚠ 12:08:11  SSH brute: root / qwerty' },
  { t: 'evt', text: '⚠ 12:08:12  SSH brute: root / 12345678' },
  { t: 'evt', text: '⚠ 12:08:13  SSH brute: ubuntu / ubuntu' },
  { t: 'evt', text: '⚠ 12:08:14  SSH brute: admin / admin' },
  { t: 'evt', text: '⚠ 12:08:15  SSH brute: git / git' },
  {
    t: 'log',
    en: '→ AI: dictionary attack, 412 attempts / 2 min',
    ru: '→ AI: словарная атака, 412 попыток / 2 мин',
  },
  {
    t: 'ok',
    en: '✓ 193.32.162.78 auto-banned in iptables',
    ru: '✓ 193.32.162.78 забанен в iptables',
  },
  { t: 'tg', en: '📩 Telegram alert sent', ru: '📩 Алерт отправлен в Telegram' },
  { t: 'pause', ms: 1500 },

  { t: 'evt', text: '⚠ 12:11:09  Redis CONFIG SET dir /var/spool/cron/' },
  { t: 'evt', text: '⚠ 12:11:09  Redis CONFIG SET dbfilename root' },
  {
    t: 'log',
    en: '→ AI: classic Redis RCE chain (Mirai-style)',
    ru: '→ AI: классическая Redis RCE-цепочка (в стиле Mirai)',
  },
  {
    t: 'ok',
    en: '✓ 92.118.160.17 auto-banned in iptables',
    ru: '✓ 92.118.160.17 забанен в iptables',
  },
  { t: 'tg', en: '📩 Telegram alert sent', ru: '📩 Алерт отправлен в Telegram' },
  { t: 'pause', ms: 1400 },

  { t: 'evt', text: '⚠ 12:14:56  HTTP POST /boaform/admin/formLogin' },
  { t: 'evt', text: '⚠ 12:14:57  HTTP GET /cgi-bin/luci/;stok=/locale' },
  {
    t: 'log',
    en: '→ AI: IoT router exploit, Mozi/Gafgyt signature',
    ru: '→ AI: эксплойт IoT-роутера, сигнатура Mozi/Gafgyt',
  },
  { t: 'pause', ms: 1200 },

  { t: 'evt', text: '⚠ 12:18:22  SMB negotiate · likely CVE-2024-21413' },
  {
    t: 'log',
    en: '→ AI: targeted recon — manual operator, not bot',
    ru: '→ AI: целевая разведка — работает человек, не бот',
  },
  {
    t: 'tg',
    en: '📩 Telegram alert sent (high severity)',
    ru: '📩 Алерт отправлен в Telegram (высокий приоритет)',
  },
  { t: 'pause', ms: 1600 },

  { t: 'evt', text: '⚠ 12:22:40  Postgres auth: postgres / postgres' },
  { t: 'evt', text: '⚠ 12:22:41  Postgres auth: postgres / admin' },
  { t: 'evt', text: '⚠ 12:22:42  Postgres COPY FROM PROGRAM attempt' },
  { t: 'log', en: '→ AI: DB takeover chain', ru: '→ AI: цепочка захвата БД' },
  {
    t: 'ok',
    en: '✓ 77.90.185.12 auto-banned in iptables',
    ru: '✓ 77.90.185.12 забанен в iptables',
  },
  { t: 'pause', ms: 1400 },

  {
    t: 'evt',
    en: '⚠ 12:27:03  SSH probe from 185.220.101.42 (Tor exit)',
    ru: '⚠ 12:27:03  SSH-зондирование с 185.220.101.42 (Tor exit)',
  },
  {
    t: 'log',
    en: '→ Same actor returning — 3rd visit today',
    ru: '→ Тот же актор возвращается — 3-й визит за день',
  },
  {
    t: 'log',
    en: '→ AI: persistence check, monitoring',
    ru: '→ AI: проверка персистентности, наблюдаю',
  },
  { t: 'pause', ms: 1600 },

  { t: 'cmd', text: '$ goronin stats --today' },
  { t: 'log', en: '→ Events:      1 284', ru: '→ Событий:      1 284' },
  { t: 'log', en: '→ Unique IPs:  137', ru: '→ Уникальных IP: 137' },
  { t: 'log', en: '→ Auto-bans:   42', ru: '→ Автобанов:    42' },
  { t: 'log', en: '→ Top trap:    SSH (71%)', ru: '→ Топ ловушка:  SSH (71%)' },
  {
    t: 'log',
    en: '→ Top country: RU, CN, NL, US',
    ru: '→ Топ страны:   RU, CN, NL, US',
  },
  {
    t: 'ok',
    en: '✓ All traps healthy · agent stable · 0 errors',
    ru: '✓ Все ловушки в норме · агент стабилен · 0 ошибок',
  },
  { t: 'pause', ms: 1800 },

  {
    t: 'log',
    en: '→ Watching. Quiet for now.',
    ru: '→ Наблюдаю. Пока тихо.',
  },
];

const TYPE_DELAY_FIRST = 600;
const TYPE_DELAY = 1000;
const MAX_LINES = 14;

const lineClass = (t) =>
  t === 'cmd'
    ? 'text-zinc-100'
    : t === 'ok'
    ? 'text-emerald-400'
    : t === 'evt'
    ? 'text-amber-400'
    : t === 'tg'
    ? 'text-sky-400'
    : 'text-zinc-500';

const resolveText = (step, lang) => step.text ?? step[lang] ?? step.en;

export default function TerminalDemo() {
  const [lang, setLang] = useState('ru');
  const [history, setHistory] = useState([]);
  const [idx, setIdx] = useState(0);
  const keyRef = useRef(0);
  const scrollRef = useRef(null);

  useEffect(() => {
    if (idx >= session.length) return;

    const step = session[idx];

    if (step.t === 'pause') {
      const id = setTimeout(() => setIdx((i) => i + 1), step.ms);
      return () => clearTimeout(id);
    }

    const delay = idx === 0 ? TYPE_DELAY_FIRST : TYPE_DELAY;
    const id = setTimeout(() => {
      setHistory((h) => {
        const next = [...h, { step, key: keyRef.current++ }];
        return next.length > MAX_LINES ? next.slice(next.length - MAX_LINES) : next;
      });
      setIdx((i) => i + 1);
    }, delay);
    return () => clearTimeout(id);
  }, [idx]);

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [history]);

  return (
    <div className="rounded-2xl bg-zinc-900/80 border border-zinc-800 overflow-hidden shadow-2xl shadow-emerald-500/5 backdrop-blur">
      <div className="flex items-center gap-2 px-4 py-3 border-b border-zinc-800 bg-zinc-900/60">
        <div className="w-3 h-3 rounded-full bg-zinc-700" />
        <div className="w-3 h-3 rounded-full bg-zinc-700" />
        <div className="w-3 h-3 rounded-full bg-zinc-700" />
        <span className="ml-3 text-xs text-zinc-500 font-mono">root@prod-web-01 ~</span>
        <div className="ml-auto flex items-center gap-1 text-[10px] font-mono">
          <button
            type="button"
            onClick={() => setLang('ru')}
            className={`px-2 py-0.5 rounded transition-colors ${
              lang === 'ru'
                ? 'bg-zinc-800 text-zinc-100'
                : 'text-zinc-500 hover:text-zinc-300'
            }`}
            aria-pressed={lang === 'ru'}
          >
            RU
          </button>
          <button
            type="button"
            onClick={() => setLang('en')}
            className={`px-2 py-0.5 rounded transition-colors ${
              lang === 'en'
                ? 'bg-zinc-800 text-zinc-100'
                : 'text-zinc-500 hover:text-zinc-300'
            }`}
            aria-pressed={lang === 'en'}
          >
            EN
          </button>
        </div>
      </div>

      <div
        ref={scrollRef}
        className="p-5 font-mono text-xs md:text-sm leading-relaxed h-[320px] overflow-hidden"
      >
        {history.map(({ step, key }) => (
          <motion.div
            key={key}
            initial={{ opacity: 0, x: -8 }}
            animate={{ opacity: 1, x: 0 }}
            transition={{ duration: 0.55, ease: 'easeOut' }}
            className={lineClass(step.t)}
          >
            {resolveText(step, lang)}
          </motion.div>
        ))}
        <span className="inline-block w-2 h-4 bg-emerald-400 animate-pulse ml-0.5 align-middle" />
      </div>
    </div>
  );
}
