import { useEffect, useState } from 'react';
import { X, Server, ArrowRight } from 'lucide-react';

const STORAGE_KEY = 'goronin-announcement-timeweb-1';
const REF_URL = 'https://timeweb.cloud/?i=104289';

export default function AnnouncementBar() {
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    if (window.localStorage.getItem(STORAGE_KEY) !== 'closed') {
      setVisible(true);
    }
  }, []);

  const close = () => {
    setVisible(false);
    try {
      window.localStorage.setItem(STORAGE_KEY, 'closed');
    } catch {
      /* empty */
    }
  };

  if (!visible) return null;

  return (
    <div className="relative bg-emerald-500/10 border-b border-emerald-500/20 text-zinc-200">
      <div className="max-w-6xl mx-auto px-5 py-2 flex items-center justify-center gap-3 text-xs sm:text-sm">
        <span className="hidden sm:inline-flex items-center justify-center w-5 h-5 rounded bg-emerald-500/15 text-emerald-300 shrink-0">
          <Server size={12} />
        </span>
        <span className="text-zinc-300">
          <span className="text-zinc-500 font-mono mr-2 hidden sm:inline">[ad]</span>
          Нужен сервер для своих сервисов?{' '}
          <a
            href={REF_URL}
            target="_blank"
            rel="sponsored noopener"
            className="text-emerald-300 hover:text-emerald-200 font-medium underline-offset-4 hover:underline inline-flex items-center gap-1"
          >
            Timeweb Cloud
            <ArrowRight size={12} />
          </a>{' '}
          <span className="text-zinc-500">— VPS, которым пользуемся сами. Поможем с настройкой.</span>
        </span>
        <button
          onClick={close}
          aria-label="Закрыть"
          className="ml-2 shrink-0 p-1 rounded hover:bg-emerald-500/10 text-zinc-500 hover:text-zinc-200 transition-colors"
        >
          <X size={14} />
        </button>
      </div>
    </div>
  );
}
