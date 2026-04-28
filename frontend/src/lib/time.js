export function timeAgo(dateStr) {
  const now = Date.now();
  const then = new Date(dateStr).getTime();
  const seconds = Math.floor((now - then) / 1000);

  if (seconds < 60) return 'только что';
  if (seconds < 3600) return `${Math.floor(seconds / 60)} мин назад`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)} ч назад`;
  if (seconds < 2592000) return `${Math.floor(seconds / 86400)} дн назад`;
  return new Date(dateStr).toLocaleDateString('ru-RU');
}
