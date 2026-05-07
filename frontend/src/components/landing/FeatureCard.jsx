import { motion } from 'framer-motion';

export default function FeatureCard({ icon: Icon, title, description, delay = 0 }) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 20 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, margin: '-60px' }}
      transition={{ duration: 0.5, delay, ease: [0.22, 1, 0.36, 1] }}
      whileHover={{ y: -4 }}
      className="group relative rounded-2xl border border-zinc-800/80 bg-zinc-900/40 p-6 hover:border-emerald-500/30 transition-colors"
    >
      <div className="absolute inset-0 rounded-2xl bg-gradient-to-b from-emerald-500/5 to-transparent opacity-0 group-hover:opacity-100 transition-opacity pointer-events-none" />
      <div className="relative">
        <div className="inline-flex p-2.5 rounded-xl bg-zinc-800/80 border border-zinc-700/50 mb-4">
          <Icon size={20} className="text-emerald-400" />
        </div>
        <h3 className="text-base font-semibold text-zinc-100 mb-1.5">{title}</h3>
        <p className="text-sm text-zinc-400 leading-relaxed">{description}</p>
      </div>
    </motion.div>
  );
}
