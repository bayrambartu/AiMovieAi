import React, { useState } from 'react';
import { api } from './api';

export default function AuthScreen({ onAuthenticated, onGuest, notice = '' }) {
  const [mode, setMode] = useState('login');
  const [form, setForm] = useState({ name: '', surname: '', email: '', password: '', confirmation: '' });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [visible, setVisible] = useState(false);
  const registering = mode === 'register';
  const change = e => setForm(current => ({ ...current, [e.target.name]: e.target.value }));
  function switchMode(next) { setMode(next); setError(''); setForm(current => ({ ...current, password: '', confirmation: '' })); }
  async function submit(e) {
    e.preventDefault();
    if (busy) return;
    setError('');
    if (registering && form.password !== form.confirmation) { setError('Şifreler eşleşmiyor.'); return; }
    setBusy(true);
    try {
      const body = registering
        ? { name: form.name, surname: form.surname, email: form.email, password: form.password }
        : { email: form.email, password: form.password };
      const user = await api('/auth/' + mode, { method: 'POST', body: JSON.stringify(body) });
      onAuthenticated(user);
    } catch (e) { setError(e.message); }
    finally { setBusy(false); }
  }
  return <div className="auth-page">
    <a className="auth-brand" href="#" onClick={e => { e.preventDefault(); if (!busy) onGuest(); }}>ai<span>movie</span>ai<span className="brand-period">.</span></a>
    <main className="auth-layout">
      <section className="auth-story" aria-label="aimovieai">
        <div className="eyebrow"><span /> HER RUH HALİNE BİR HİKÂYE</div>
        <h1>İyi hikâyeler.<br /><em>Sana ait bir dünya.</em></h1>
        <p>Sevdiğin filmleri bir araya getir. Kaldığın yerden devam et.<br />Bir sonraki favorini birlikte bulalım.</p>
        <div className="auth-ticket" aria-hidden="true"><span>ADMIT ONE · AIMOVIEAI</span><strong>Senin zevkin.<br />Senin sineman.</strong><div className="auth-ticket-line" /><small>KEŞFET / KAYDET / YENİDEN İZLE</small></div>
        <p className="auth-story-caption">Bir filmle başlar. Bir koleksiyona dönüşür.</p>
      </section>
      <section className="auth-card" aria-labelledby="auth-title">
        <div className="auth-tabs" aria-label="Hesap işlemi"><button type="button" aria-pressed={!registering} className={!registering ? 'active' : ''} disabled={busy} onClick={() => switchMode('login')}>Giriş yap</button><button type="button" aria-pressed={registering} className={registering ? 'active' : ''} disabled={busy} onClick={() => switchMode('register')}>Hesap oluştur</button></div>
        <div className="eyebrow small">{registering ? 'YENİ BİR BAŞLANGIÇ' : 'KALDIĞIN YERDEN'}</div>
        <h2 id="auth-title">{registering ? 'Kendi film dünyanı kur.' : 'Tekrar hoş geldin.'}</h2>
        <p>{registering ? 'Listelerin ve tercihlerinin tümü bir hesapta.' : 'Film listen ve yeni keşiflerin seni bekliyor.'}</p>
        {notice && <p className="auth-notice" role="status">{notice}</p>}
        <form onSubmit={submit}>
          <fieldset disabled={busy}>
            {registering && <div className="auth-name-row"><label>Ad<input autoComplete="given-name" name="name" value={form.name} onChange={change} maxLength={100} required /></label><label>Soyad<input autoComplete="family-name" name="surname" value={form.surname} onChange={change} maxLength={100} required /></label></div>}
            <label>E-posta<input autoComplete="email" type="email" name="email" value={form.email} onChange={change} maxLength={254} placeholder="sen@ornek.com" required /></label>
            <label>Şifre<div className="auth-password"><input autoComplete={registering ? 'new-password' : 'current-password'} type={visible ? 'text' : 'password'} name="password" value={form.password} onChange={change} minLength={registering ? 8 : undefined} maxLength={72} required aria-describedby={registering ? 'password-help' : undefined} /><button type="button" aria-label={visible ? 'Şifreyi gizle' : 'Şifreyi göster'} aria-pressed={visible} onClick={() => setVisible(v => !v)}>{visible ? 'Gizle' : 'Göster'}</button></div></label>
            {registering && <><small id="password-help" className="auth-help">En az 8 karakter; en fazla 72 bayt.</small><label>Şifre tekrar<input autoComplete="new-password" type={visible ? 'text' : 'password'} name="confirmation" value={form.confirmation} onChange={change} minLength={8} maxLength={72} required /></label></>}
            {error && <div className="auth-error" role="alert">{error}</div>}
            <button className="primary auth-submit" type="submit" disabled={busy}>{busy ? <><span className="spinner" /> Lütfen bekle…</> : registering ? 'Hesabımı oluştur →' : 'Giriş yap →'}</button>
          </fieldset>
        </form>
        {registering && <p className="auth-footnote">Yeni hesabın boş bir film listesiyle başlar.</p>}
        <button className="auth-guest" type="button" disabled={busy} onClick={onGuest}>Şimdilik misafir olarak keşfet ↗</button>
      </section>
    </main>
    <footer className="auth-footer"><span>İyi bir film, her şeyi değiştirir.</span><span>TMDB’den hikâyeler. Sana özel bir seçim.</span></footer>
  </div>;
}
