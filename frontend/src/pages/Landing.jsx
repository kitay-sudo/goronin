import { motion, AnimatePresence } from 'framer-motion';
import {
  Terminal,
  Bell,
  Brain,
  FileWarning,
  Lock,
  Github,
  Gauge,
  Network,
  Swords,
  Copy,
  Check,
  ArrowUp,
  Heart,
  Send,
  Shield,
  Eye,
  History,
  Sparkles,
} from 'lucide-react';
import { useState, useEffect } from 'react';
import GridBackground from '../components/landing/GridBackground';
import Reveal from '../components/landing/Reveal';
import TerminalDemo from '../components/landing/TerminalDemo';
import FeatureCard from '../components/landing/FeatureCard';
import FAQItem from '../components/landing/FAQItem';
import RoninMark from '../components/landing/RoninMark';
import KanjiWatermark from '../components/landing/KanjiWatermark';
import JapaneseDivider from '../components/landing/JapaneseDivider';

const REPO_URL = 'https://github.com/kitay-sudo/goronin';
const INSTALL_CMD = 'curl -sSL https://raw.githubusercontent.com/kitay-sudo/goronin/main/install.sh | sudo bash';

export default function Landing() {
  return (
    <div className="min-h-dvh bg-zinc-950 text-zinc-100 antialiased">
      <Nav />
      <Hero />
      <RoninStory />
      <LogosStrip />
      <Features />
      <Versus />
      <HowItWorks />
      <DemoSection />
      <FAQ />
      <Support />
      <CTA />
      <Changelog />
      <Footer />
      <BackToTop />
    </div>
  );
}

function BackToTop() {
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    const onScroll = () => setVisible(window.scrollY > 400);
    onScroll();
    window.addEventListener('scroll', onScroll, { passive: true });
    return () => window.removeEventListener('scroll', onScroll);
  }, []);

  const scrollUp = () =>
    window.scrollTo({ top: 0, behavior: 'smooth' });

  return (
    <AnimatePresence>
      {visible && (
        <motion.button
          key="back-to-top"
          type="button"
          onClick={scrollUp}
          aria-label="Наверх"
          initial={{ opacity: 0, y: 12 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: 12 }}
          transition={{ duration: 0.2, ease: 'easeOut' }}
          className="fixed z-50 bottom-6 right-6 inline-flex items-center justify-center w-11 h-11 rounded-full border border-emerald-500/30 bg-zinc-900/80 backdrop-blur text-emerald-400 hover:text-emerald-300 hover:border-emerald-500/60 hover:bg-zinc-900 shadow-lg shadow-emerald-500/10 transition-colors"
        >
          <ArrowUp size={18} strokeWidth={2.2} />
        </motion.button>
      )}
    </AnimatePresence>
  );
}

function Nav() {
  return (
    <header className="sticky top-0 z-50 border-b border-zinc-900/80 bg-zinc-950/70 backdrop-blur-lg">
      <div className="max-w-6xl mx-auto px-5 h-14 flex items-center justify-between">
        <a href="#top" className="flex items-center gap-2.5 font-semibold">
          <span className="inline-flex items-center justify-center w-8 h-8 rounded-md border border-emerald-500/30 bg-emerald-500/10 text-emerald-400">
            <RoninMark size={22} />
          </span>
          <span className="tracking-tight">GORONIN</span>
        </a>

        <nav className="hidden md:flex items-center gap-7 text-sm text-zinc-400">
          <a href="#way" className="hover:text-zinc-100 transition-colors">Путь</a>
          <a href="#features" className="hover:text-zinc-100 transition-colors">Возможности</a>
          <a href="#how" className="hover:text-zinc-100 transition-colors">Как работает</a>
          <a href="#install" className="hover:text-zinc-100 transition-colors">Установка</a>
          <a href="#faq" className="hover:text-zinc-100 transition-colors">FAQ</a>
          <a href="#changelog" className="hover:text-zinc-100 transition-colors">Изменения</a>
          <a href="#support" className="text-amber-300/90 hover:text-amber-200 transition-colors inline-flex items-center gap-1.5">
            <Heart size={12} fill="currentColor" />
            Стена чести
          </a>
        </nav>

        <a
          href={REPO_URL}
          target="_blank"
          rel="noreferrer"
          className="text-sm font-medium bg-emerald-500 hover:bg-emerald-400 text-zinc-950 rounded-lg px-3.5 py-1.5 transition-colors flex items-center gap-1.5"
        >
          <Github size={14} />
          GitHub
        </a>
      </div>
    </header>
  );
}

function Hero() {
  return (
    <section id="top" className="relative overflow-hidden">
      <GridBackground />

      <KanjiWatermark
        char="牢"
        className="right-[3%] top-[12%] text-[180px] md:text-[260px] hidden sm:block"
        target={0.045}
      />
      <KanjiWatermark
        char="人"
        className="right-[3%] top-[40%] text-[180px] md:text-[260px] hidden sm:block"
        target={0.045}
      />
      <KanjiWatermark
        char="守"
        className="left-[4%] top-[20%] text-[160px] md:text-[220px] hidden md:block"
        target={0.03}
      />

      <div className="relative max-w-6xl mx-auto px-5 pt-20 md:pt-28 pb-20 md:pb-32">
        <Reveal>
          <div className="flex justify-center">
            <div className="inline-flex items-center gap-2 px-3 py-1 rounded-full border border-zinc-800 bg-zinc-900/60 text-xs text-zinc-400">
              <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse" />
              <span className="font-serif-jp text-zinc-500">浪人</span>
              <span className="h-3 w-px bg-zinc-700" />
              Honeypot + детект взлома + авто-бан IP · MIT
            </div>
          </div>
        </Reveal>

        <Reveal delay={0.05}>
          <h1 className="mt-6 text-center text-4xl md:text-6xl font-semibold tracking-tight leading-[1.05]">
            Страж без хозяина.<br />
            <span className="bg-gradient-to-r from-emerald-400 to-teal-300 bg-clip-text text-transparent">
              Молча ждёт. Вовремя бьёт.
            </span>
          </h1>
        </Reveal>

        <Reveal delay={0.08}>
          <p className="mt-6 text-center text-lg md:text-xl text-zinc-200 max-w-2xl mx-auto leading-relaxed font-medium">
            Расставляет ловушки на сервере, ловит сканеры и взломщиков в реальном времени,
            мгновенно шлёт алерт в Telegram и сам банит атакующий IP в iptables.
          </p>
        </Reveal>

        <Reveal delay={0.16}>
          <p className="mt-5 text-center text-sm md:text-base text-zinc-400 max-w-2xl mx-auto leading-relaxed">
            Один Go-бинарь на сервер. Опционально — AI-разбор атаки (Claude / GPT / Gemini, на ваш ключ).
            Без бэкенда, без аккаунтов, полностью open-source.
          </p>
        </Reveal>

        <Reveal delay={0.15}>
          <div className="mt-8 max-w-2xl mx-auto">
            <InstallCommand />
          </div>
        </Reveal>

        <Reveal delay={0.2}>
          <div className="mt-5 flex flex-col sm:flex-row items-center justify-center gap-3">
            <a
              href={REPO_URL}
              target="_blank"
              rel="noreferrer"
              className="w-full sm:w-auto flex items-center justify-center gap-2 text-zinc-300 hover:text-zinc-100 border border-zinc-800 hover:border-zinc-700 rounded-xl px-5 py-3 transition-colors"
            >
              <Github size={16} />
              Исходники на GitHub
            </a>
            <a
              href="#how"
              className="w-full sm:w-auto flex items-center justify-center gap-2 text-zinc-400 hover:text-zinc-200 transition-colors px-5 py-3"
            >
              <Terminal size={16} />
              Как работает
            </a>
          </div>
        </Reveal>

        <Reveal delay={0.25}>
          <div className="mt-6 text-center text-xs text-zinc-500">
            Бесплатно навсегда · Полный код открыт · MIT-лицензия · Работает на любом Linux с systemd
          </div>
        </Reveal>

        <motion.div
          initial={{ opacity: 0, y: 30 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.8, delay: 0.3, ease: [0.22, 1, 0.36, 1] }}
          className="mt-14 md:mt-20 max-w-3xl mx-auto"
        >
          <TerminalDemo />
        </motion.div>
      </div>
    </section>
  );
}

function InstallCommand() {
  const [copied, setCopied] = useState(false);
  const onCopy = async () => {
    try {
      await navigator.clipboard.writeText(INSTALL_CMD);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      /* empty */
    }
  };

  return (
    <div className="rounded-2xl border border-emerald-500/30 bg-zinc-900/80 backdrop-blur p-4 shadow-lg shadow-emerald-500/10">
      <div className="flex items-center justify-between gap-3">
        <code className="text-xs sm:text-sm text-emerald-300 font-mono break-all flex-1 min-w-0">
          {INSTALL_CMD}
        </code>
        <button
          onClick={onCopy}
          className="shrink-0 inline-flex items-center gap-1.5 text-xs font-medium px-2.5 py-1.5 rounded-md border border-zinc-700 hover:border-emerald-500/50 hover:bg-emerald-500/10 transition-colors text-zinc-300"
          aria-label="Скопировать команду"
        >
          {copied ? <Check size={14} className="text-emerald-400" /> : <Copy size={14} />}
          {copied ? 'Скопировано' : 'Копировать'}
        </button>
      </div>
    </div>
  );
}

function RoninStory() {
  return (
    <section id="way" className="relative border-y border-zinc-900/80 overflow-hidden">
      <KanjiWatermark
        char="道"
        className="left-[50%] top-[50%] -translate-x-1/2 -translate-y-1/2 text-[280px] md:text-[440px]"
        target={0.035}
      />

      <div className="relative max-w-4xl mx-auto px-5 py-20 md:py-28 text-center">
        <Reveal>
          <JapaneseDivider kanji="道" label="The Way" />
          <h2 className="text-3xl md:text-5xl font-semibold tracking-tight leading-tight">
            Почему <span className="text-emerald-400">ронин</span>?
          </h2>
        </Reveal>

        <Reveal delay={0.1}>
          <p className="mt-6 text-zinc-400 leading-relaxed md:text-lg">
            Ронин — 浪人 — самурай без хозяина. Скитающийся воин, не связанный приказами,
            действующий по своему кодексу.
          </p>
        </Reveal>

        <Reveal delay={0.15}>
          <p className="mt-4 text-zinc-400 leading-relaxed md:text-lg">
            GORONIN (<span className="text-emerald-400 font-mono">Go + ronin</span>) —
            агент-страж, который ставишь на сервер и забываешь. Он сам мониторит, сам ловит, сам блокирует.
            Не ходит на чужие серверы за командами — только локальный демон и исходящие алерты.
          </p>
        </Reveal>

        <Reveal delay={0.2}>
          <div className="mt-10 grid grid-cols-1 md:grid-cols-3 gap-4">
            {[
              { kanji: '黙', label: 'Молчание', desc: 'Бинарь ~10 МБ, < 30 МБ RAM. Не шумит в логах, не нагружает CPU.' },
              { kanji: '速', label: 'Скорость', desc: 'Алерт в Telegram через секунды после первого коннекта на ловушку.' },
              { kanji: '義', label: 'Верность', desc: 'Никаких бэкендов и аккаунтов. Только локально и исходящие HTTPS.' },
            ].map((v) => (
              <div
                key={v.kanji}
                className="rounded-2xl border border-zinc-800/80 bg-zinc-900/40 p-5 text-left"
              >
                <div className="flex items-center gap-3 mb-2">
                  <span
                    className="text-2xl text-zinc-600"
                    style={{ fontFamily: '"Noto Serif JP", serif', fontWeight: 500 }}
                  >
                    {v.kanji}
                  </span>
                  <span className="text-sm font-semibold text-zinc-100">{v.label}</span>
                </div>
                <p className="text-sm text-zinc-400 leading-relaxed">{v.desc}</p>
              </div>
            ))}
          </div>
        </Reveal>
      </div>
    </section>
  );
}

function LogosStrip() {
  const items = ['Ubuntu', 'Debian', 'CentOS', 'Alpine', 'Rocky', 'Arch'];
  return (
    <section className="bg-zinc-950">
      <div className="max-w-6xl mx-auto px-5 py-8">
        <p className="text-center text-xs uppercase tracking-widest text-zinc-600 mb-5">
          Работает на любом Linux с systemd
        </p>
        <div className="flex flex-wrap items-center justify-center gap-x-10 gap-y-4 opacity-70">
          {items.map((x) => (
            <span key={x} className="text-sm font-medium text-zinc-500">{x}</span>
          ))}
        </div>
      </div>
    </section>
  );
}

function Features() {
  const features = [
    {
      icon: Network,
      title: 'Ловушки на портах',
      description: 'SSH, HTTP, FTP, MySQL — на случайных high-портах. Любой коннект = аномалия = алерт.',
    },
    {
      icon: FileWarning,
      title: 'File canary',
      description: 'inotify-мониторинг чувствительных файлов (.env, id_rsa) и подкинутых приманок в /root, /tmp.',
    },
    {
      icon: Bell,
      title: 'Telegram-алерты',
      description: 'Свой бот, свой chat. Подробное событие + цепочка атак при score ≥ 50.',
    },
    {
      icon: Brain,
      title: 'AI на выбор',
      description: 'Claude, GPT-4o или Gemini — твой ключ, твой счёт. Можно не подключать вообще.',
    },
    {
      icon: Gauge,
      title: 'Авто-бан в iptables',
      description: 'Threshold + эскалация: 3 хита за 5 мин — бан на час, повтор — на сутки. Persistent.',
    },
    {
      icon: Lock,
      title: 'Zero trust by design',
      description: 'Никакого центрального сервера. Все секреты — у тебя на машине. Полностью open-source.',
    },
  ];

  return (
    <section id="features" className="relative py-24 md:py-32 border-t border-zinc-900/80">
      <div className="max-w-6xl mx-auto px-5">
        <Reveal>
          <div className="max-w-2xl mx-auto text-center">
            <JapaneseDivider kanji="技" label="Capabilities" />
            <h2 className="text-3xl md:text-4xl font-semibold tracking-tight">
              Всё, что нужно для раннего обнаружения вторжений
            </h2>
            <p className="mt-4 text-zinc-400 leading-relaxed">
              Атаки на серверы начинаются не с эксплойта, а со сканирования. GORONIN ловит именно этот момент —
              когда кто-то только пытается найти слабое место.
            </p>
          </div>
        </Reveal>

        <div className="mt-14 grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {features.map((f, i) => (
            <FeatureCard key={f.title} {...f} delay={i * 0.05} />
          ))}
        </div>
      </div>
    </section>
  );
}

function Versus() {
  const rows = [
    {
      label: 'Что триггерит бан',
      goronin: 'Первый же коннект на порт-приманку — туда легитимный трафик не ходит',
      classic: 'N неудачных попыток в логе боевого сервиса (sshd, nginx)',
    },
    {
      label: 'Когда ловит атакующего',
      goronin: 'На стадии разведки — пока он только сканирует порты',
      classic: 'Когда уже стучится в реальный SSH / веб с подбором пароля',
    },
    {
      label: 'False positives',
      goronin: 'Близко к нулю — на honeypot-порту не бывает «своих»',
      classic: 'Бывают: опечатался в пароле — забанил себя',
    },
    {
      label: 'Мониторинг файлов',
      goronin: 'Да — inotify на .env, id_rsa и подкинутые канарейки',
      classic: 'Нет — только парсинг логов',
    },
    {
      label: 'Алерты в Telegram',
      goronin: 'Из коробки, с опциональным AI-разбором атаки',
      classic: 'Пилится руками через action.d',
    },
    {
      label: 'Защита публичного SSH/nginx',
      goronin: 'Не покрывает — это не его задача',
      classic: 'Базовый сценарий, для которого fail2ban и сделан',
    },
  ];

  return (
    <section className="relative py-24 md:py-32 border-t border-zinc-900/80 overflow-hidden">
      <KanjiWatermark
        char="比"
        className="left-[3%] top-[15%] text-[160px] md:text-[240px] hidden md:block"
        target={0.03}
      />

      <div className="relative max-w-5xl mx-auto px-5">
        <Reveal>
          <div className="text-center max-w-2xl mx-auto">
            <JapaneseDivider kanji="比" label="The Comparison" />
            <h2 className="text-3xl md:text-4xl font-semibold tracking-tight">
              GORONIN vs fail2ban
            </h2>
            <p className="mt-4 text-zinc-400 leading-relaxed">
              Сам механизм бана у нас тот же — iptables. Разница в том,{' '}
              <span className="text-zinc-200">что именно</span> заставляет систему сработать. Это не замена
              fail2ban, а другой класс защиты. Хорошо стоят вместе.
            </p>
          </div>
        </Reveal>

        <Reveal delay={0.1}>
          <div className="mt-12 grid grid-cols-1 md:grid-cols-2 gap-px bg-zinc-900/80 rounded-2xl overflow-hidden border border-zinc-900/80">
            <div className="bg-zinc-950 p-5 md:p-6 border-b md:border-b-0 md:border-r border-zinc-900/80">
              <div className="flex items-center gap-2.5 mb-1">
                <span className="inline-flex items-center justify-center w-8 h-8 rounded-md border border-emerald-500/30 bg-emerald-500/10 text-emerald-400">
                  <Shield size={16} />
                </span>
                <span className="text-base font-semibold text-zinc-100">GORONIN</span>
                <span className="text-xs text-zinc-500 font-mono ml-auto">honeypot-first</span>
              </div>
              <p className="text-sm text-zinc-500">Ловит на стадии разведки — до того, как взломщик дошёл до боевого сервиса.</p>
            </div>
            <div className="bg-zinc-950 p-5 md:p-6">
              <div className="flex items-center gap-2.5 mb-1">
                <span className="inline-flex items-center justify-center w-8 h-8 rounded-md border border-zinc-700 bg-zinc-900 text-zinc-400">
                  <Eye size={16} />
                </span>
                <span className="text-base font-semibold text-zinc-100">fail2ban / CrowdSec</span>
                <span className="text-xs text-zinc-500 font-mono ml-auto">log-first</span>
              </div>
              <p className="text-sm text-zinc-500">Парсит логи реальных сервисов и банит после порога неудачных попыток.</p>
            </div>
          </div>
        </Reveal>

        <Reveal delay={0.15}>
          <div className="mt-6 overflow-hidden rounded-2xl border border-zinc-900/80 divide-y divide-zinc-900/80">
            <div className="hidden md:grid md:grid-cols-[200px_1fr_1fr] bg-zinc-950/60">
              <div className="px-5 py-3 border-r border-zinc-900/80" />
              <div className="px-5 py-3 text-xs uppercase tracking-wider text-emerald-400 font-semibold border-r border-zinc-900/80">
                GORONIN
              </div>
              <div className="px-5 py-3 text-xs uppercase tracking-wider text-zinc-400 font-semibold">
                fail2ban / CrowdSec
              </div>
            </div>
            {rows.map((r) => (
              <div
                key={r.label}
                className="grid grid-cols-1 md:grid-cols-[200px_1fr_1fr] bg-zinc-950"
              >
                <div className="px-5 py-4 md:py-5 text-xs uppercase tracking-wider text-zinc-500 font-medium md:border-r border-zinc-900/80 flex md:items-center">
                  {r.label}
                </div>
                <div className="px-5 pt-1 pb-2 md:py-5 text-sm text-zinc-200 md:border-r border-zinc-900/80 leading-relaxed">
                  <span className="md:hidden block text-[10px] uppercase tracking-wider text-emerald-400 font-semibold mb-1">GORONIN</span>
                  {r.goronin}
                </div>
                <div className="px-5 pt-1 pb-5 md:py-5 text-sm text-zinc-400 leading-relaxed">
                  <span className="md:hidden block text-[10px] uppercase tracking-wider text-zinc-400 font-semibold mb-1">fail2ban / CrowdSec</span>
                  {r.classic}
                </div>
              </div>
            ))}
          </div>
        </Reveal>

        <Reveal delay={0.2}>
          <p className="mt-8 text-center text-sm text-zinc-500 max-w-2xl mx-auto leading-relaxed">
            Сценарий, в котором это работает идеально: fail2ban сторожит публичный SSH и веб,
            а GORONIN ловит того, кто сканирует порты в поисках чего ещё открыто. Два разных слоя.
          </p>
        </Reveal>
      </div>
    </section>
  );
}

function HowItWorks() {
  const steps = [
    {
      num: '01',
      title: 'Запусти install.sh',
      description: 'Одна команда от root. Скрипт скачает бинарь под твою arch и запустит wizard.',
    },
    {
      num: '02',
      title: 'Ответь на вопросы',
      description: 'Telegram bot token + chat_id, AI-провайдер (опционально), какие ловушки включить, whitelist IP.',
    },
    {
      num: '03',
      title: 'Получай алерты',
      description: 'Wizard поднимет systemd-сервис. Через минуту — тестовое сообщение в Telegram. Готово.',
    },
  ];

  return (
    <section id="how" className="relative py-24 md:py-32 border-t border-zinc-900/80">
      <div className="max-w-6xl mx-auto px-5">
        <Reveal>
          <div className="text-center max-w-2xl mx-auto">
            <JapaneseDivider kanji="歩" label="The Path" />
            <h2 className="text-3xl md:text-4xl font-semibold tracking-tight">
              Три шага до защиты
            </h2>
          </div>
        </Reveal>

        <div className="mt-14 grid grid-cols-1 md:grid-cols-3 gap-5 md:gap-8 relative">
          <div className="hidden md:block absolute top-9 left-[16%] right-[16%] h-px bg-gradient-to-r from-transparent via-zinc-800 to-transparent" />
          {steps.map((s, i) => (
            <Reveal key={s.num} delay={i * 0.08}>
              <div className="relative">
                <div className="w-[72px] h-[72px] rounded-2xl border border-zinc-800 bg-zinc-900/60 backdrop-blur flex items-center justify-center mb-5 mx-auto">
                  <span className="text-2xl font-mono font-semibold text-zinc-300 tracking-tight">
                    {s.num}
                  </span>
                </div>
                <h3 className="text-lg font-semibold text-center text-zinc-100 mb-2">{s.title}</h3>
                <p className="text-sm text-zinc-400 leading-relaxed text-center max-w-xs mx-auto">
                  {s.description}
                </p>
              </div>
            </Reveal>
          ))}
        </div>

        <Reveal delay={0.2}>
          <div id="install" className="mt-14 max-w-2xl mx-auto">
            <InstallCommand />
            <p className="mt-3 text-center text-xs text-zinc-500">
              После установки доступны команды: <code className="text-zinc-400">goronin status | logs -f | restart | unban &lt;ip&gt; | reconfigure</code>
            </p>
          </div>
        </Reveal>
      </div>
    </section>
  );
}

function DemoSection() {
  return (
    <section className="relative py-24 md:py-32 border-t border-zinc-900/80 overflow-hidden">
      <div className="absolute inset-0 pointer-events-none">
        <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[700px] h-[400px] bg-emerald-500/5 blur-[120px] rounded-full" />
      </div>
      <KanjiWatermark
        char="眼"
        className="right-[2%] top-[10%] text-[160px] md:text-[240px] hidden md:block"
        target={0.03}
      />

      <div className="relative max-w-3xl mx-auto px-5 text-center">
        <Reveal>
          <JapaneseDivider kanji="眼" label="The Eye" />
          <h2 className="text-3xl md:text-4xl font-semibold tracking-tight leading-tight">
            Что приходит в Telegram
          </h2>
          <p className="mt-4 text-zinc-400 leading-relaxed">
            Откуда пришла атака, на какие порты ломились, в какое время — и что агент уже сделал
            (заблокировал IP или просто записал). Если подключён AI — добавляется короткий разбор
            на русском: бот это или человек, что искали, насколько опасно.
          </p>
          <p className="mt-3 text-zinc-500 leading-relaxed text-sm">
            Когда один IP пытается несколько разных вещей подряд — приходит одно сообщение со всей
            цепочкой, а не десять отдельных.
          </p>
        </Reveal>
      </div>
    </section>
  );
}

function FAQ() {
  const items = [
    {
      q: 'Это правда полностью open-source? Никакого SaaS?',
      a: 'Да. Один Go-бинарь, MIT-лицензия. Нет бэкенда, нет аккаунтов, нет телеметрии. Все ключи (Telegram bot, AI provider) — твои собственные, лежат на твоём сервере в /etc/goronin/config.yml (mode 0600).',
    },
    {
      q: 'Что именно делает агент?',
      a: '1) Поднимает honeypot-ловушки на случайных high-портах (SSH/HTTP/FTP/MySQL). 2) Через inotify следит за чувствительными файлами и созданными канарейками. 3) При попадании в ловушку — пишет в локальный bbolt, считает hits per IP, банит через iptables (с эскалацией). 4) Шлёт алерт в Telegram, опционально с AI-разбором.',
    },
    {
      q: 'Заменяет ли это fail2ban / CrowdSec?',
      a: 'Нет, и не пытается. fail2ban парсит логи реальных сервисов (sshd, nginx) и банит после N неудачных попыток — это его работа. GORONIN ловит другой класс атак: разведку и сканирование портов, до того как взломщик дошёл до боевого сервиса. Сам бан в обоих случаях идёт через iptables. Идеальный сетап — оба вместе: fail2ban охраняет публичный SSH и веб, GORONIN ловит сканеры на портах-приманках.',
    },
    {
      q: 'Какой AI-провайдер выбрать?',
      a: 'Любой из трёх: Anthropic Claude, OpenAI GPT-4o, Google Gemini. Wizard спросит при установке. Можно вообще без AI — алерты будут приходить, просто без объяснительного абзаца.',
    },
    {
      q: 'Безопасно ли запускать curl | sudo bash?',
      a: 'Скрипт короткий, читай его перед запуском: github.com/kitay-sudo/goronin/blob/main/install.sh. Он только определяет архитектуру, скачивает бинарь из GitHub Releases и запускает интерактивный wizard. Никаких внешних серверов кроме github.com.',
    },
    {
      q: 'Влияет ли агент на производительность?',
      a: 'Бинарь ~10 МБ, RAM в простое < 30 МБ, CPU около нуля. Ловушки — обычные TCP-listeners на high-портах, ничего тяжёлого. bbolt-файл состояния занимает килобайты.',
    },
    {
      q: 'Что если я не хочу автобан?',
      a: 'В wizard выбери mode = "off" (только алерты, iptables не трогается) или "alert_only" (логировать что забанилось бы, но не банить — dry-run для первой недели). Permission-mode "enforce" — production-режим с реальным баном.',
    },
    {
      q: 'А если сервер перезагрузится?',
      a: 'Активные баны и счётчики hits переживают reboot — всё хранится в /var/lib/goronin/state.db (bbolt). systemd запустит сервис автоматически.',
    },
    {
      q: 'Можно ли поставить на несколько серверов?',
      a: 'Да, ставь на любое количество. Каждый сервер — независимый агент со своим конфигом и своим Telegram chat (можно один и тот же chat для всех — алерты будут содержать имя сервера).',
    },
  ];

  return (
    <section id="faq" className="py-24 md:py-32 border-t border-zinc-900/80">
      <div className="max-w-3xl mx-auto px-5">
        <Reveal>
          <div className="text-center">
            <JapaneseDivider kanji="問" label="Questions" />
            <h2 className="text-3xl md:text-4xl font-semibold tracking-tight">
              Ответы на самое важное
            </h2>
          </div>
        </Reveal>

        <Reveal delay={0.1}>
          <div className="mt-10">
            {items.map((it) => (
              <FAQItem key={it.q} question={it.q} answer={it.a} />
            ))}
          </div>
        </Reveal>
      </div>
    </section>
  );
}

// Стена донатеров. Чтобы добавить нового — допиши объект в массив и сделай commit.
// Поля:
//   handle    — ник (с @ или без, при отрисовке @ всё равно срезается)
//   amount    — строка для отображения, например "0.66 TON" или "10 USDT"
//   amountTon — число в TON для сортировки (используется ТОЛЬКО для определения «top»)
//   addedAt   — дата в ISO ("2026-04-28"), нужна для роли first-ever (первая десятка)
//   note      — опционально, короткая ремарка
//
// Роли расставляются автоматически:
//   • TOP DONOR  — у кого amountTon максимален (золотой акцент)
//   • FIRST EVER — первые 10 по addedAt (серебряный шильдик, не отбирается)
const DONORS = [
  {
    handle: 'sounee',
    amount: '0.658894824 TON',
    amountTon: 0.658894824,
    addedAt: '2026-04-28',
    note: 'один из первой десятки поддержавших',
  },
];

const TELEGRAM_HANDLE = '@kitay9';
const TELEGRAM_URL = 'https://t.me/kitay9';

function Support() {
  const wallets = [
    {
      label: 'USDT',
      network: 'TRON · TRC20',
      address: 'TF9F2FPkreHVfbe8tZtn4V76j3jLo4SeXM',
    },
    {
      label: 'TON',
      network: 'The Open Network',
      address: 'UQBl88kXWJWyHkDPkWNYQwwSCiCAIfA2DiExtZElwJFlIc1o',
    },
  ];

  return (
    <section id="support" className="relative py-24 md:py-32 border-t border-zinc-900/80 overflow-hidden">
      <KanjiWatermark
        char="恩"
        className="left-[5%] top-[20%] text-[160px] md:text-[240px] hidden md:block"
        target={0.03}
      />

      <div className="relative max-w-3xl mx-auto px-5">
        <Reveal>
          <div className="text-center">
            <JapaneseDivider kanji="恩" label="Gratitude" />
            <h2 className="text-3xl md:text-4xl font-semibold tracking-tight">
              Поддержать проект
            </h2>
            <p className="mt-4 text-zinc-400 leading-relaxed max-w-xl mx-auto">
              GORONIN развивается на энтузиазме и в свободное время. Если он оказался полезен — поддержать можно криптой.
              Любая сумма — это вклад в инструмент, которым удобно администрировать всем нам:
              новые ловушки, фичи из roadmap и время на их доведение.
            </p>
            <p className="mt-3 text-zinc-500 text-sm leading-relaxed max-w-xl mx-auto">
              Поддержавшие попадают в{' '}
              <a href="#support" className="text-amber-300/90 hover:text-amber-200 underline-offset-4 hover:underline">
                стену чести
              </a>{' '}
              ниже — это публичный список тех, кто помог проекту встать на ноги.
            </p>
          </div>
        </Reveal>

        <Reveal delay={0.1}>
          <div className="mt-10 grid grid-cols-1 md:grid-cols-2 gap-4">
            {wallets.map((w, i) => (
              <WalletCard key={w.label} {...w} delay={i * 0.05} />
            ))}
          </div>
        </Reveal>

        <Reveal delay={0.15}>
          <div className="mt-10 rounded-2xl border border-emerald-500/20 bg-gradient-to-br from-emerald-500/5 via-zinc-900/40 to-zinc-900/40 p-6 md:p-8">
            <div className="flex items-start gap-4">
              <div className="shrink-0 inline-flex items-center justify-center w-11 h-11 rounded-xl border border-emerald-500/30 bg-emerald-500/10 text-emerald-400">
                <Send size={18} />
              </div>
              <div className="flex-1 min-w-0">
                <h3 className="text-base md:text-lg font-semibold text-zinc-100">
                  Хочешь попасть в стену чести?
                </h3>
                <p className="mt-1.5 text-sm text-zinc-400 leading-relaxed">
                  После доната напиши в Telegram{' '}
                  <a
                    href={TELEGRAM_URL}
                    target="_blank"
                    rel="noreferrer"
                    className="text-emerald-400 hover:text-emerald-300 font-mono"
                  >
                    {TELEGRAM_HANDLE}
                  </a>{' '}
                  свой ник — добавлю в список ниже навсегда.
                </p>
                <a
                  href={TELEGRAM_URL}
                  target="_blank"
                  rel="noreferrer"
                  className="mt-4 inline-flex items-center gap-1.5 text-xs font-medium px-3 py-2 rounded-md border border-emerald-500/30 hover:border-emerald-500/60 hover:bg-emerald-500/10 transition-colors text-emerald-300"
                >
                  <Send size={13} />
                  Написать в Telegram
                </a>
              </div>
            </div>
          </div>
        </Reveal>

        <Reveal delay={0.2}>
          <DonorsWall donors={DONORS} />
        </Reveal>
      </div>
    </section>
  );
}

function DonorsWall({ donors }) {
  const empty = !donors || donors.length === 0;

  // Определяем особые роли: top (по сумме) и first-ever (по дате).
  // Один и тот же донатер может держать обе роли — в этом случае показываем одну
  // карточку с двумя плашками. Когда придёт следующий с большей суммой — он
  // станет top, а sounee останется first-ever, и они разъедутся в две карточки.
  const annotated = !empty ? annotateDonors(donors) : [];
  const honors = annotated.filter((d) => d.roles.length > 0);
  const rest = annotated.filter((d) => d.roles.length === 0);

  return (
    <div className="mt-10">
      <div className="flex items-center gap-3 mb-2">
        <Heart size={14} className="text-emerald-400" strokeWidth={2.4} />
        <h3 className="text-sm font-semibold tracking-wide uppercase text-zinc-300">
          Стена чести
        </h3>
        {!empty && (
          <span className="text-xs text-zinc-500 font-mono ml-auto">
            {donors.length} {donors.length === 1 ? 'самурай' : 'самураев'}
          </span>
        )}
      </div>
      {!empty && (
        <p className="mb-5 text-xs text-zinc-500 leading-relaxed">
          <span className="text-amber-300/90 font-mono">一番 top</span> — крупнейший донат на сейчас.{' '}
          <span className="text-zinc-300 font-mono">初 first ever</span> — первая десятка тех, кто
          поддержал проект раньше всех. Этот шильдик не отбирается и достаётся только им — отдельная
          каста ранних, навсегда.
        </p>
      )}

      {empty ? (
        <div className="rounded-xl border border-dashed border-zinc-800 bg-zinc-900/30 p-8 text-center">
          <p className="text-sm text-zinc-500">
            Пока пусто.{' '}
            <a
              href={TELEGRAM_URL}
              target="_blank"
              rel="noreferrer"
              className="text-emerald-400 hover:text-emerald-300 font-medium"
            >
              Будь первым
            </a>{' '}
            — твой ник окажется здесь и останется навсегда.
          </p>
        </div>
      ) : (
        <div className="space-y-5">
          {honors.length > 0 && (
            <div
              className={
                honors.length === 1
                  ? 'grid grid-cols-1 gap-4'
                  : 'grid grid-cols-1 md:grid-cols-2 gap-4'
              }
            >
              {honors.map((d) => (
                <HonorCard key={d.handle} donor={d} />
              ))}
            </div>
          )}

          {rest.length > 0 && (
            <ul className="flex flex-wrap gap-2">
              {rest.map((d) => {
                const nick = d.handle.replace(/^@/, '');
                return (
                  <li
                    key={nick}
                    title={d.note || nick}
                    className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full border border-zinc-800 bg-zinc-900/50 text-sm text-zinc-300 font-mono"
                  >
                    <Heart size={11} className="text-emerald-400/70" fill="currentColor" />
                    {nick}
                    {d.amount && (
                      <span className="text-zinc-500 text-[11px]">· {d.amount}</span>
                    )}
                  </li>
                );
              })}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}

// Сколько ранних донатеров получают серебряный шильдик "first ever".
const FIRST_EVER_LIMIT = 10;

// Помечает донатеров ролями top / first-ever. Если кто-то держит обе — обе и получит.
function annotateDonors(donors) {
  let topIdx = -1;
  let topAmount = -Infinity;
  donors.forEach((d, i) => {
    const a = typeof d.amountTon === 'number' ? d.amountTon : -Infinity;
    if (a > topAmount) {
      topAmount = a;
      topIdx = i;
    }
  });

  // Первые N по дате — отдельная каста, ранг не отбирается.
  const firstIdxs = new Set(
    donors
      .map((d, i) => ({ i, t: d.addedAt ? new Date(d.addedAt).getTime() : NaN }))
      .filter((x) => !Number.isNaN(x.t))
      .sort((a, b) => a.t - b.t)
      .slice(0, FIRST_EVER_LIMIT)
      .map((x) => x.i),
  );

  return donors.map((d, i) => {
    const roles = [];
    if (i === topIdx && topAmount > -Infinity) roles.push('top');
    if (firstIdxs.has(i)) roles.push('first-ever');
    return { ...d, roles };
  });
}

function HonorCard({ donor }) {
  const nick = donor.handle.replace(/^@/, '');
  const isTop = donor.roles.includes('top');
  const isFirst = donor.roles.includes('first-ever');

  // Если донатер держит «top» — карточка золотая. Иначе (только first-ever) — серебряная.
  const gold = isTop;
  const palette = gold
    ? {
        border: 'border-amber-500/30',
        bg: 'from-amber-500/10 via-zinc-900/60 to-zinc-900/40',
        iconWrap: 'border-amber-500/40 bg-amber-500/10 text-amber-300',
      }
    : {
        border: 'border-zinc-400/25',
        bg: 'from-zinc-300/5 via-zinc-900/60 to-zinc-900/40',
        iconWrap: 'border-zinc-400/40 bg-zinc-300/10 text-zinc-200',
      };

  return (
    <div
      title={donor.note || nick}
      className={`relative block overflow-hidden rounded-2xl border bg-gradient-to-br p-5 md:p-6 ${palette.border} ${palette.bg}`}
    >
      <div className="relative flex items-start gap-4">
        <div
          className={`shrink-0 inline-flex items-center justify-center w-12 h-12 rounded-xl border ${palette.iconWrap}`}
        >
          <Heart size={20} fill="currentColor" />
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            {isTop && (
              <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full border border-amber-500/40 bg-amber-500/10 text-amber-300 text-[10px] uppercase tracking-widest font-semibold">
                <span style={{ fontFamily: '"Noto Serif JP", serif', fontWeight: 700 }}>
                  一番
                </span>
                Top
              </span>
            )}
            {isFirst && (
              <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full border border-zinc-400/40 bg-zinc-300/10 text-zinc-200 text-[10px] uppercase tracking-widest font-semibold">
                <span style={{ fontFamily: '"Noto Serif JP", serif', fontWeight: 700 }}>
                  初
                </span>
                First ever
              </span>
            )}
          </div>

          <div className="mt-2 flex flex-wrap items-baseline gap-x-3 gap-y-1">
            <span className="text-xl md:text-2xl font-semibold font-mono text-zinc-100">
              {nick}
            </span>
            {donor.amount && (
              <span className="text-sm font-mono text-emerald-300">{donor.amount}</span>
            )}
          </div>

          {(donor.note || (isTop && isFirst)) && (
            <p className="mt-1.5 text-xs text-zinc-500 leading-relaxed">
              {isTop && isFirst
                ? donor.note || 'и первый по времени, и пока крупнейший донат'
                : isTop
                  ? donor.note || 'крупнейший донат на момент сейчас'
                  : donor.note || 'один из первой десятки поддержавших'}
            </p>
          )}
        </div>
      </div>
    </div>
  );
}

function WalletCard({ label, network, address }) {
  const [copied, setCopied] = useState(false);
  const onCopy = async () => {
    try {
      await navigator.clipboard.writeText(address);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      /* empty */
    }
  };

  return (
    <div className="rounded-2xl border border-zinc-800 bg-zinc-900/40 p-5 backdrop-blur">
      <div className="flex items-baseline justify-between mb-3">
        <span className="text-base font-semibold text-zinc-100">{label}</span>
        <span className="text-xs text-zinc-500 font-mono">{network}</span>
      </div>
      <div className="flex items-center gap-2 rounded-lg border border-zinc-800 bg-zinc-950/60 p-3">
        <code className="text-xs text-emerald-300 font-mono break-all flex-1 min-w-0">
          {address}
        </code>
        <button
          onClick={onCopy}
          className="shrink-0 inline-flex items-center gap-1.5 text-xs font-medium px-2.5 py-1.5 rounded-md border border-zinc-700 hover:border-emerald-500/50 hover:bg-emerald-500/10 transition-colors text-zinc-300"
          aria-label={`Скопировать адрес ${label}`}
        >
          {copied ? <Check size={14} className="text-emerald-400" /> : <Copy size={14} />}
          {copied ? 'Скопировано' : 'Копировать'}
        </button>
      </div>
    </div>
  );
}

function CTA() {
  return (
    <section className="relative py-24 md:py-32 border-t border-zinc-900/80 overflow-hidden">
      <div className="absolute inset-0 pointer-events-none">
        <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[700px] h-[400px] bg-emerald-500/10 blur-[120px] rounded-full" />
      </div>
      <div className="relative max-w-3xl mx-auto px-5 text-center">
        <Reveal>
          <div className="inline-flex items-center justify-center w-14 h-14 rounded-xl border border-emerald-500/30 bg-emerald-500/10 text-emerald-400 mb-6">
            <Swords size={26} />
          </div>
          <h2 className="text-3xl md:text-5xl font-semibold tracking-tight leading-tight">
            Поставь стража.<br />
            <span className="bg-gradient-to-r from-emerald-400 to-teal-300 bg-clip-text text-transparent">
              Узнай, кто ломится к тебе сейчас.
            </span>
          </h2>
          <p className="mt-5 text-zinc-400 max-w-xl mx-auto">
            60 секунд от curl до первого алерта в Telegram.
          </p>

          <div className="mt-8 max-w-2xl mx-auto">
            <InstallCommand />
          </div>

          <div className="mt-5 flex items-center justify-center">
            <a
              href={REPO_URL}
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-2 text-zinc-300 hover:text-zinc-100 border border-zinc-800 hover:border-zinc-700 rounded-xl px-5 py-3 transition-colors"
            >
              <Github size={16} />
              Посмотреть код на GitHub
            </a>
          </div>
        </Reveal>
      </div>
    </section>
  );
}

// Журнал релизов. Чтобы добавить запись — допиши объект в начало массива
// и сделай commit. Самая верхняя запись автоматически получает плашку "latest".
//   version    — строка вида "0.2.0"
//   date       — ISO-дата релиза
//   title      — короткий заголовок одной строкой
//   highlights — 2-4 буллита с самым важным; не нужно копировать весь CHANGELOG
const CHANGELOG = [
  {
    version: '0.2.0',
    date: '2026-04-28',
    title: 'Двухуровневая агрегация алертов',
    highlights: [
      'Сводный алерт на скан вместо 30 отдельных — AI-вызовов в сутки 5–10 вместо 200+',
      'Новые тунаблы urgent_window / background_window / interest_threshold в config.yml',
      'Мгновенная ветка для file canary — write/remove идут мимо агрегатора',
      'HOW_IT_WORKS.md в корне репо с расчётами стоимости',
    ],
  },
  {
    version: '0.1.0',
    date: '2026-04-27',
    title: 'Первый open-source релиз',
    highlights: [
      'Полностью standalone — один Go-бинарь, без бэкенда и аккаунтов',
      'AI-провайдеры на выбор: Claude / GPT / Gemini, опционально',
      'Persistent state через bbolt — баны переживают reboot',
      'install.sh: curl | sudo bash → wizard → systemd-сервис за минуту',
    ],
  },
];

const RELEASES_URL = 'https://github.com/kitay-sudo/goronin/releases';
const CHANGELOG_URL = 'https://github.com/kitay-sudo/goronin/blob/main/CHANGELOG.md';

function Changelog() {
  const dateFormatter = new Intl.DateTimeFormat('ru-RU', {
    day: 'numeric',
    month: 'long',
    year: 'numeric',
  });

  return (
    <section id="changelog" className="relative py-24 md:py-32 border-t border-zinc-900/80 overflow-hidden">
      <KanjiWatermark
        char="記"
        className="right-[5%] top-[15%] text-[160px] md:text-[240px] hidden md:block"
        target={0.03}
      />

      <div className="relative max-w-3xl mx-auto px-5">
        <Reveal>
          <div className="text-center">
            <JapaneseDivider kanji="記" label="The Chronicle" />
            <h2 className="text-3xl md:text-4xl font-semibold tracking-tight">
              Журнал изменений
            </h2>
            <p className="mt-4 text-zinc-400 leading-relaxed max-w-xl mx-auto">
              Что меняется в каждом релизе и когда он был выпущен — чтобы было видно,
              что проект живой и в каком направлении движется.
            </p>
          </div>
        </Reveal>

        <Reveal delay={0.1}>
          <ol className="mt-12 relative border-l border-zinc-800/80 ml-3">
            {CHANGELOG.map((rel, idx) => {
              const isLatest = idx === 0;
              const dateLabel = (() => {
                const d = new Date(rel.date);
                return Number.isNaN(d.getTime()) ? rel.date : dateFormatter.format(d);
              })();

              return (
                <li key={rel.version} className="relative pl-8 pb-10 last:pb-0">
                  <span
                    className={`absolute -left-[7px] top-1.5 w-3.5 h-3.5 rounded-full border-2 ${
                      isLatest
                        ? 'border-emerald-400 bg-emerald-500/30 shadow-[0_0_0_4px_rgba(16,185,129,0.08)]'
                        : 'border-zinc-700 bg-zinc-900'
                    }`}
                    aria-hidden
                  />

                  <div className="flex flex-wrap items-center gap-2 mb-1">
                    <span className="text-base md:text-lg font-mono font-semibold text-zinc-100">
                      v{rel.version}
                    </span>
                    {isLatest && (
                      <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full border border-emerald-500/40 bg-emerald-500/10 text-emerald-300 text-[10px] uppercase tracking-widest font-semibold">
                        <Sparkles size={10} />
                        Latest
                      </span>
                    )}
                    <span className="text-xs text-zinc-500 font-mono ml-auto">
                      <time dateTime={rel.date}>{dateLabel}</time>
                    </span>
                  </div>

                  <h3 className="text-sm md:text-base font-semibold text-zinc-200 mb-3">
                    {rel.title}
                  </h3>

                  <ul className="space-y-1.5">
                    {rel.highlights.map((h, i) => (
                      <li
                        key={i}
                        className="flex gap-2 text-sm text-zinc-400 leading-relaxed"
                      >
                        <span className="shrink-0 mt-2 w-1 h-1 rounded-full bg-zinc-600" />
                        <span>{h}</span>
                      </li>
                    ))}
                  </ul>
                </li>
              );
            })}
          </ol>
        </Reveal>

        <Reveal delay={0.2}>
          <div className="mt-10 flex flex-wrap items-center justify-center gap-3">
            <a
              href={CHANGELOG_URL}
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-2 text-sm text-zinc-300 hover:text-zinc-100 border border-zinc-800 hover:border-zinc-700 rounded-lg px-4 py-2 transition-colors"
            >
              <History size={14} />
              Полный CHANGELOG
            </a>
            <a
              href={RELEASES_URL}
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-2 text-sm text-zinc-300 hover:text-zinc-100 border border-zinc-800 hover:border-zinc-700 rounded-lg px-4 py-2 transition-colors"
            >
              <Github size={14} />
              Все релизы на GitHub
            </a>
          </div>
        </Reveal>
      </div>
    </section>
  );
}

function Footer() {
  return (
    <footer className="border-t border-zinc-900/80 py-10">
      <div className="max-w-6xl mx-auto px-5 flex flex-col md:flex-row items-center justify-between gap-4">
        <div className="flex items-center gap-2 text-sm text-zinc-500">
          <span>GORONIN · MIT · © {new Date().getFullYear()}</span>
          <span className="text-zinc-700">·</span>
          <a
            href="https://github.com/kitay-sudo"
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1 text-zinc-500 hover:text-emerald-400 transition-colors"
          >
            by <Github size={12} /> kitay-sudo
          </a>
        </div>
        <div className="flex items-center gap-5 text-sm text-zinc-500">
          <a href="#features" className="hover:text-zinc-300 transition-colors">Возможности</a>
          <a href="#install" className="hover:text-zinc-300 transition-colors">Установка</a>
          <a href="#faq" className="hover:text-zinc-300 transition-colors">FAQ</a>
          <a href="#changelog" className="hover:text-zinc-300 transition-colors">Изменения</a>
          <a href="#support" className="hover:text-zinc-300 transition-colors">Поддержать</a>
          <a href={REPO_URL} target="_blank" rel="noreferrer" className="hover:text-zinc-300 transition-colors flex items-center gap-1.5">
            <Github size={14} /> GitHub
          </a>
        </div>
      </div>
    </footer>
  );
}
