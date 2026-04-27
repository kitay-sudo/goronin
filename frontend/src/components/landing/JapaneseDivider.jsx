export default function JapaneseDivider({ kanji, label }) {
  return (
    <div className="flex items-center justify-center gap-3 mb-4">
      <span className="h-px w-10 bg-gradient-to-r from-transparent to-zinc-700" />
      <span
        className="text-zinc-600 text-lg"
        style={{
          fontFamily: '"Noto Serif JP", "Yu Mincho", "Hiragino Mincho ProN", serif',
          fontWeight: 500,
        }}
      >
        {kanji}
      </span>
      <span className="text-[11px] tracking-[0.25em] uppercase text-zinc-500 font-medium">
        {label}
      </span>
      <span className="h-px w-10 bg-gradient-to-l from-transparent to-zinc-700" />
    </div>
  );
}
