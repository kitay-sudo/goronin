import { useState } from 'react';
import { Copy, Check } from 'lucide-react';

export default function CopyButton({ text }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    await navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <button
      onClick={handleCopy}
      className="p-2 rounded-lg text-zinc-400 hover:text-zinc-100 hover:bg-zinc-800 transition-colors"
      title="Copy"
    >
      {copied ? <Check size={16} className="text-emerald-400" /> : <Copy size={16} />}
    </button>
  );
}
