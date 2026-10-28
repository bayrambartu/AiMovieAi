# GitHub preparation checks — 2026-10-29

- The local application was started with make dev. PostgreSQL was healthy; frontend and API returned HTTP 200, with qwen3:4b available in Ollama.
- Backend unit/HTTP tests, Go race detection and vet passed. PostgreSQL integration tests also passed against a disposable database, which was removed afterwards; application data was preserved.
- Frontend production build and both development/preview proxy regression tests passed.
- CI workflow YAML syntax/job structure and staged-file whitespace were checked locally. Remote GitHub Actions execution has not yet been verified.
- The actual staged content was checked for the configured TMDB token, JWT-like values, private keys, local personal paths and operational account records; no matches remained. .env, local backups, dependencies and generated builds are excluded from Git.
- Detailed operational history was retained in an ignored local backup; the public notes contain technical verification only.

---

# Serbest konu önerileri — 2026-10-23

- Kök hata doğrulandı: yapılandırılmış qwen2.5:3b kurulu değildi; model hatası sabit yerel konu kurallarına sessizce düşüyordu. Etkin metin yorumlama artık LLM arama planını kullanır; konu kabul listesi yoktur.
- Yerel son model qwen3:4b, think:false, 4096 bağlam. Daha eski küçük modellerin Türkçe ayrıntı kaybı ve 8 GB RAM’de 7B yükleme zaman aşımı canlı incelendi. Kullanılmayan bu oturumluk qwen2.5:3b/7b indirmeleri kaldırıldı; önceden mevcut llama3.2 ve bge-m3 korundu.
- Son kod: İngilizce kısa arama planı, etiket yokluğunda alternatif adaylar, gerçek başlık çözümü, en fazla sekiz adayın İngilizce kanıtı, kaynak parçası indeksleri, Türkçe görünümün korunması. Ana konuya doğrudan kanıt varsa yalnız ikincil ipucuna dayalı sonuçlar ayıklanır; konuya özel kelime eklenmedi.
- Ayrı PostgreSQL test kümesinde tüm 31 ana test race ile geçti; go vet ve frontend üretim derlemesi başarılı. Test veritabanı kapatıldı. Çalışan API yeni kodla yeniden başlatıldı; frontend health yanıtı model qwen3:4b ve status ok.
- Son canlı testler http://127.0.0.1:5173/api/recommendations üzerinden Origin/Sec-Fetch-Site başlıklarıyla yapıldı:
  - “zombi filmi öner”: HTTP 200, status ok, 8 aday, model_overview; Zombieland (19908), Resident Evil (1423191), Resident Evil: Raccoon Şehri (460458), Dünya Savaşı Z (72190). Kaynak gerekçeleri gerçek zombie etiketi. Yaklaşık 65.4 saniye.
  - Deniz fenerinde yalnız yaşayan biri: HTTP 200, status ok, 8 aday; Deniz Feneri (503919). Gerçek İngilizce özet lighthouse keepers/remote island bilgisini içeriyor. Önceki yalnız island kelimesiyle seçilen ek sonuçlar son akışta yok. Yaklaşık 51.3 saniye. İstenen tek kişi/filmdeki iki bekçi ayrımı bu örneğin kusursuz tüm koşul eşleşmesi kanıtı değildir; ana konu/ortam eşleşmesi doğrulandı.
- Bunlar iki tekil canlı örnektir; tüm konular için kalite garantisi veya tüm TMDB kataloğunu tarama kanıtı değildir. Görsel/tarayıcı tıklama kontrolü yapılmadı. Testler film listelerine ekleme yapmadı.

Ayrıntı: [FREEFORM-RECOMMENDATIONS.md](FREEFORM-RECOMMENDATIONS.md). Son yerel test çıktısı `.local/freeform-live-results.json` içindedir.

---

# Empty account collections

Registration never imports guest activities into a new account. The integration suite verifies an empty initial collection, preserved guest data, explicit account saves, and user isolation. Operational account cleanup records are not part of the public repository.

---

# Hesap oluşturma Origin/proxy düzeltmesi — 2026-10-22

- Curl ile doğrudan API’de doğrulamaya ulaşan kayıt isteğinin frontend proxy üzerinden 403 aldığı yeniden üretildi.
- Vite kısa proxy yazımının Host’u değiştirdiği kurulu kaynak kodundan doğrulandı. Dev ve preview için `changeOrigin: false` ayarlandı; backend kontrolü korunuyor.
- İki sentetik hesapla 8080 ve 5173 üzerinden kayıt 201, profil/giriş/çıkış 200, çıkış sonrası profil 401; farklı-Origin istek 403.
- `npm run test:proxy`: geliştirme ve preview için 2 regresyon testi geçti. Frontend üretim derlemesi başarılı.
- Detay ve tekrar komutu: [CSRF-PROXY.md](CSRF-PROXY.md). Tarayıcıdaki elle tıklama/görsel kontrolü doğrulanmadı.

---

# Kullanıcı yönetimi doğrulaması — 2026-10-22

- Ayrı yerel PostgreSQL 18 test kümesinde `TEST_DATABASE_URL=... go test -race -v ./...`: **21 ana test ve 4 alt senaryo geçti**; PostgreSQL testleri atlanmadı. Mevcut uygulama veritabanı kullanılmadı.
- `TestPostgresAuthFlow`: kayıt/giriş/profil/çıkış, bcrypt şifre saklama, normalize e-posta benzersizliği, token özeti, misafir liste aktarımı, farklı cihazdan erişim, token yenileme, kullanıcılar arası tercih izolasyonu, hesap bazlı izlendi filtresi ve süre sonu doğrulandı.
- Eski `TestPostgresFlow` ilk gerçek koşuda, sahte detay verisinde space kanıtı bulunmamasına rağmen uzay önerisi beklediği için başarısız oldu. Testin amacı olan migration/kalıcılık/tür-tercih filtresini koruyarak istek yalnız bilim kurgu türüyle sınırlandı; tüm testler geçti. Üretim öneri algoritması değiştirilmedi.
- `go vet ./...`: başarılı. Geçici test sunucusunda gerçek HTTP kontrolü: `/api/health` 200, `/` 200 HTML, oturumsuz `/api/auth/me` 401.
- Frontend: giriş/kayıt ekranı, profil adı ve çıkış, oturum kontrolü, misafir keşif ve kimlik değişiminde uygulama durumunun temizlenmesi entegre edildi. Vite üretim derlemesi başarılı; 25 modül.
- Tarayıcı kontrolü denendi; tarayıcı bağlantısı bulunmadı ve native Chrome için bilgisayar kullanım izni verilmedi. Görsel/tarayıcı etkileşimleri doğrulanmadı. Canlı TMDB/Ollama kalitesi bu kapsamda sınanmadı.
- API testleri TMDB/Ollama için sahte taşıyıcılar kullanır; gerçek kullanıcı veya kaynak movie-list hesabı oluşturulmadı/taşınmadı.

Ayrıntı: [AUTH.md](AUTH.md).

---

## Önceki doğrulama kaydı (kullanıcı yönetiminden önce)

# Tema ve özet değerlendirmesi doğrulaması

- `go test -race -v ./...`: **18 ana test ve 4 alt senaryo geçti**. Gerçek PostgreSQL entegrasyonu `TEST_DATABASE_URL` olmadığı için atlandı.
- Yeni regresyonlar: “başarı anlatan film öner” tür zorlamadan başarı/azim temasına dönüşür; sentetik test havuzundaki yüksek puanlı Kara Şövalye ve başarılı dedektif hikâyesi elenir, kişisel hedef/azim hikâyesi kalır. Bu test canlı model kalitesi ölçümü değildir.
- Belirsiz/gibberish istek hiçbir PostgreSQL veya TMDB çağrısı yapmadan açıklama sorusu döndürür. Çözülemeyen TMDB etiketi geniş popüler keşfe dönmez. Korku dışlama, izlenenler, puan ve birden fazla tema kontrolleri doğrulandı.
- Modelin uydurduğu kimlikler ve film verisinde olmayan kanıt alıntıları reddedilir. Boş semantik sonuçlar popüler filmlerle doldurulmaz. Model aşamaları kontrollü sahte HTTP yanıtlarıyla test edildi.
- `go vet ./...` ve Go derlemesi başarılı.
- **Vite 6.4.1 üretim derlemesi başarılı:** 23 modül, yaklaşık 207 kB JS ve 19 kB CSS. `frontend/dist` üretildi.
- Kabuktaki varsayılan Node 14 olduğundan ilk npm derlemesi sözdizimi hatası verdi. Bilgisayarda mevcut `/opt/homebrew/bin/node` (25.9.0) ile derleme geçti. Yeni `scripts/with-node.sh`, geliştirme/Makefile komutlarında mevcut desteklenen Node'u seçer; yeni runtime indirmez ve sistem ayarını değiştirmez.
- Frontend artık katalog ve öneri için ayrı tür seçimi kullanır. Eski öneriler yeni istek sırasında temizlenir; netleştirme sorusu, yorumlanan tema ve isteği düzenleme eylemi eklendi. Gerçek tarayıcı etkileşimleri bu oturumda doğrulanmadı.
- Önceki ortam kontrollerinde Docker socket ve canlı TMDB/Ollama erişimi kısıtlıydı. Bu değişiklikte gerçek model/TMDB/PostgreSQL ile uçtan uca kalite doğrulaması yapılmadı; servisler çalışırken yeniden denenmeli.

Başlatmak için `make dev`; mevcut süreç açıksa durdurup yeniden başlatın. AI modeli için README'deki Ollama ayarları geçerlidir. Yeni veritabanı migration'ı veya film tablosu eklenmedi.
