import { Loader2 } from 'lucide-react';

export default function Spinner({ size = 24, className = '' }) {
  return (
    <div className={`flex items-center justify-center ${className}`}>
      <Loader2 size={size} className="animate-spin-slow text-emerald-400" />
    </div>
  );
}

export function FullSpinner() {
  return (
    <div className="flex items-center justify-center min-h-[60dvh]">
      <Loader2 size={32} className="animate-spin-slow text-emerald-400" />
    </div>
  );
}
