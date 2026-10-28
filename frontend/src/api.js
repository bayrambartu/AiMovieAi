let identityVersion = 0;

export async function api(path, options = {}) {
  const startedWithIdentity = identityVersion;
  const res = await fetch('/api' + path, {
    ...options,
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', ...options.headers },
  });
  let data;
  try { data = await res.json(); }
  catch { throw new Error('Sunucuya ulaşılamadı. Go API’sinin çalıştığından emin ol.'); }
  if (!res.ok) {
    if (res.status === 401 && startedWithIdentity === identityVersion && !path.startsWith('/auth/')) window.dispatchEvent(new Event('auth-expired'));
    const error = new Error(data.error || 'İstek tamamlanamadı.');
    error.status = res.status;
    throw error;
  }
  if (['/auth/login', '/auth/register', '/auth/logout'].includes(path)) identityVersion++;
  return data;
}
