import { motion } from 'framer-motion';

export default function KanjiWatermark({ char, className = '', initial = 0, target = 0.04 }) {
  const finalOpacity = target * 0.5;
  return (
    <motion.span
      initial={{ opacity: initial }}
      animate={{ opacity: finalOpacity }}
      transition={{ duration: 1.8, ease: 'easeOut' }}
      className={`pointer-events-none select-none absolute text-zinc-400 ${className}`}
      style={{
        fontFamily: '"Noto Serif JP", "Yu Mincho", "Hiragino Mincho ProN", serif',
        fontWeight: 700,
        lineHeight: 1,
      }}
      aria-hidden
    >
      {char}
    </motion.span>
  );
}
