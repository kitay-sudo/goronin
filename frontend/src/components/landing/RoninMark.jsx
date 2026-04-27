export default function RoninMark({ size = 28, className = '' }) {
  return (
    <span
      className={`inline-flex items-center justify-center font-semibold tracking-tight ${className}`}
      style={{
        width: size,
        height: size,
        fontSize: size * 0.62,
        lineHeight: 1,
        fontFamily: '"Noto Serif JP", "Yu Mincho", "Hiragino Mincho ProN", serif',
      }}
      aria-label="Goronin"
    >
      浪
    </span>
  );
}
