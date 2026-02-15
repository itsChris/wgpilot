export function formatBytes(bytes: number): string {
  if (!bytes || bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
}

export function formatRelativeTime(unixSeconds: number): string {
  if (!unixSeconds) return 'Never';
  const now = Math.floor(Date.now() / 1000);
  const diff = now - unixSeconds;
  if (diff < 60) return 'Just now';
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return `${Math.floor(diff / 86400)}d ago`;
}

export function modeLabel(mode: string): string {
  switch (mode) {
    case 'gateway':
      return 'VPN Gateway';
    case 'site-to-site':
      return 'Site-to-Site';
    case 'hub-routed':
      return 'Hub with Peer Routing';
    default:
      return mode;
  }
}
