import { Swords } from 'lucide-react';

export default function RoninMark({ size = 28, className = '' }) {
  return (
    <span
      className={`inline-flex items-center justify-center ${className}`}
      style={{ width: size, height: size }}
      aria-label="Goronin"
    >
      <Swords size={Math.round(size * 0.7)} strokeWidth={2.2} />
    </span>
  );
}
