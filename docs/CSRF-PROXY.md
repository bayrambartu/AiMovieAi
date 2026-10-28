# Hesap oluşturma 403 — Vite proxy düzeltmesi

2026-10-22: Kullanıcı hesap oluştururken “Siteler arası istek reddedildi” hatasını bildirdi.

## Neden

Backend POST/PUT isteklerinde Origin host'u ile HTTP Host'u karşılaştırır. Vite 6.4.1'de proxy için `'/api': 'http://127.0.0.1:8080'` kısa yazımı `changeOrigin: true` olarak işlenir (kurulu Vite kaynak kodunda doğrulandı). Böylece tarayıcıdan gelen `Origin: http://127.0.0.1:5173` kalırken Host `127.0.0.1:8080` olur ve geçerli frontend isteği reddedilir.

`frontend/vite.config.js` geliştirme ve preview için açık proxy nesnesi ve `changeOrigin: false` kullanıyor. Host tarayıcı adresi olarak korunur. Backend Origin veya Sec-Fetch-Site kontrolü gevşetilmedi; frontend'in Auth.jsx → api.js → `/api/auth/register` yolu ve JSON alanları doğruydu.

## Curl ile doğrulama

Önce hesap açmadan sınır kontrolü yapıldı: `{}` gönderimi doğrudan 8080 API'de, Origin yokken veya API ile eşleşirken 400 doğrulama yanıtı verdi. Frontend 5173 üzerinden aynı-Origin başlıklı istek düzeltmeden önce 403; düzeltmeden sonra 400 verdi.

İki sentetik `@example.invalid` hesabıyla curl kayıt/giriş/profil/çıkış testi hem 8080 hem 5173 üzerinden yapıldı:

- Kayıt 201; HttpOnly oturum çerezi curl cookie jar ile yeniden kullanıldı.
- Profil 200; çıkış 200; çıkış sonrası profil 401.
- Yeniden giriş 200; son çıkış 200.
- Frontend proxy üzerinden `Origin: https://evil.example` yine 403.

Test oturumlarından çıkıldı ve geçici JSON/cookie dosyaları temizlendi. Sentetik test hesapları veritabanında bırakıldı; gerçek kullanıcı bilgileri kullanılmadı.

Yalnız Origin/proxy kontrolü için hesap oluşturmayan tekrar örneği (beklenen 400, 403 değil):

```sh
curl -i http://127.0.0.1:5173/api/auth/register \
  -H 'Content-Type: application/json' \
  -H 'Origin: http://127.0.0.1:5173' \
  -H 'Sec-Fetch-Site: same-origin' \
  --data '{}'
```

## Regresyon testi

`frontend/proxy.test.js` gerçek Vite geliştirme/preview proxy'lerini boş yerel portlarda çalıştırır. Upstream test sunucusu Origin/Host eşleşmesini kontrol eder; doğru isteğin doğrulamaya ulaştığını, farklı-Origin isteğinin reddedildiğini sınar. İki test geçti. Çalıştırma:

```sh
cd frontend
../scripts/with-node.sh npm run test:proxy
```

Frontend üretim derlemesi de başarılı. Çalışan Vite config değişikliğini otomatik yükledi; elle tarayıcı etkileşimi bu çalışmada doğrulanmadı. Farklı ortamda eski proxy süreci kalmışsa geliştirme/preview sunucusunu yeniden başlatın.
