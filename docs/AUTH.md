# Kullanıcı yönetimi

2026-10-22: movie-list projesindeki ad/soyad/e-posta/şifre kayıt, giriş ve profil akışları incelenerek aimovieai'nin mevcut net/http ve database/sql yapısına eklendi. Kaynak proje değiştirilmedi; verileri ve hesapları kopyalanmadı. Bu uygulamada oturumlar sunucuda iptal edilebilir rastgele token ve HttpOnly çerezle yönetilir.

## Akış

1. Açılışta `/api/auth/me` kontrol edilir. Geçerli oturum uygulamayı açar; 401 giriş panelini gösterir. Servis hatası ayrı tekrar deneme ekranıdır.
2. Kayıt ad, soyad, e-posta, şifre ve arayüzde şifre tekrarı içerir. E-posta trim + küçük harf ile normalleştirilir. Benzersizlik SQL ile korunur. Şifre bcrypt DefaultCost kullanır (en az 8 karakter, en fazla 72 bayt).
3. Yeni hesaplar boş film listesiyle başlar. Kayıt veya giriş sırasında misafir tercihleri hesaba aktarılmaz; misafir kayıtları kendi oturumlarında korunur. Kullanıcının açık düzeltmesiyle otomatik aktarım kaldırıldı.
4. Hesap tercihleri mevcut activities tablosunda `session_id=user:<id>` anahtarıyla saklanır; gerçek kimlik yalnız doğrulanmış sunucu bağlamından gelir. Eski 64 karakterli misafir anahtarları korunur. Bu yaklaşım mevcut tercih şemasını ve misafir kullanımını korur.
5. Girişte yeni token üretilir, aynı tarayıcıdan gönderilen eski oturum iptal edilir. Başka cihazların oturumları korunur. Çıkış mevcut token'ı veritabanından siler; hesabın film listesi kalır. Oturum 30 gün sonra geçersizdir.
6. Arayüzde kullanıcı adı/baş harfi ve çıkış düğmesi vardır. Kullanıcı değiştiğinde App yeniden oluşturulur; eski liste, detay, öneriler temizlenir. Süresi dolan hesap istekleri giriş ekranını açar. Eski kimlikle başlayan HTTP isteklerinin geç gelen 401 yanıtları yeni giriş durumunu bozmaz.

## Saklama ve sınırlar

- `users`: id, name, surname, normalize edilmiş benzersiz email, password_hash, created_at.
- `auth_sessions`: token_hash (SHA-256), user_id foreign key, expires_at, created_at. Token kriptografik rastgele 32 bayttır; ham token sadece HttpOnly çerezde bulunur.
- API profil alanları id, name, surname, email, created_at; hiçbir şifre alanı yoktur.
- SameSite=Lax; doğrudan TLS isteğinde Secure. POST/PUT için cross-site Sec-Fetch-Site veya farklı Origin reddedilir. Üretimde TLS/ters proxy davranışı ayrıca ele alınmalıdır; X-Forwarded-* başlıklarına otomatik güvenilmez.
- Kayıt ve giriş toplamında IP başına 15 dakikada 20 deneme; süreç belleğinde sınırlı tablo. Yeniden başlatmada sıfırlanır, çoklu sunucu için ortak limit değildir. Doğrudan RemoteAddr kullanılır.
- E-posta doğrulama, şifre sıfırlama/değiştirme, profil düzenleme, yönetici rolleri ve movie-list hesap aktarımı uygulanmadı.

## Doğrulama

`backend/auth_test.go` yanlış giriş/kayıt verisini, bilinmeyen alanları, bcrypt bayt sınırını, çerez bayraklarını, sahte oturum kimliğini, Origin kontrolünü ve deneme sınırını sınar. Gerçek PostgreSQL senaryosu `TEST_DATABASE_URL` ile ayrı geçici schema açar ve yalnız bu schemayı temizler: kayıt/benzersiz e-posta, bcrypt saklama, token digest, misafir verilerinden bağımsız boş hesap başlangıcı, açık kaydetme sonrası başka cihazdan erişim, token yenileme, kullanıcı izolasyonu, hesap bazlı izlendi filtresi, çıkış ve süre sonu.

Örnek (yalnız ayrı test veritabanında):

```sh
cd backend
TEST_DATABASE_URL='postgres://.../aimovieai_test?sslmode=disable' go test -race -v ./...
go vet ./...
```

Tarayıcı etkileşimlerinin elle kontrolü: kayıt → kullanıcı adını gör → listene film ekle → çıkış → yanlış şifre hatasını gör → doğru giriş → aynı listeyi gör → sayfayı yenile → çıkış. Mobil genişlikte form, kayıt/giriş geçişleri ve şifre gösterme düğmesini de kontrol et. Bu oturumda bilgisayar kullanım izni olmadığı için görsel/tarayıcı etkileşim kontrolü tamamlanamadı.

## Geliştirme proxy bağlantısı

Vite dev/preview proxy’si `changeOrigin: false` kullanmalıdır; backend Origin ile tarayıcıya görünen Host’u karşılaştırır. Vite string proxy yazımı Host’u değiştirdiğinden hesap isteklerini 403 yapar. Curl sonuçları ve regresyon testi: [CSRF-PROXY.md](CSRF-PROXY.md).
