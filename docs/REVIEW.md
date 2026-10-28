# Kaynak proje incelemesi

## MoveiHub

README, `lib/tmdb.js`, `app/api/chatbot/route.js`, `components/Hero.jsx` ve `components/MovieCard.jsx` incelendi. Next.js/React ile TMDB üzerinden katalog, film detayları ve OpenAI tabanlı Endonezce sohbet sunuyor. TMDB anahtarını `NEXT_PUBLIC_` ile tarayıcıya taşıyor. Chatbot, istemciden gelen geçmiş üzerinde doğrudan `.map` çağırıyor; yanıtları katalogdaki film kimlikleriyle eşleştirmiyor. Bazı hata akışları boş listeye dönüşerek gerçek hatayı gizleyebiliyor. Hero'daki fragman düğmesinin işleyicisi yok.

aimovieai'de keşif, detay ve öneri deneyimi yeniden uygulandı. API ve kalıcılık Go'ya taşındı; istemciye gizli anahtar gönderilmiyor. Model yalnızca tercih etiketleri üretiyor; gösterilen filmler katalogdan geliyor. Fragman/yayın gibi mevcut verinin desteklemediği işlevler eklenmedi.

## MovieDude

README, `engine.py`, `find_user_interests.py`, `find_by_title.py`, `watched_movies.py` ve SQLite şeması incelendi. `Movies_sorted` tablosunda 6.000 gerçek film mevcut. Tür ve anahtar kelimeler multi-label encoding sonrası Jaccard ile karşılaştırılıyor. İzlenenleri çıkarma ve kullanıcı puan/beğenilerinden tercih çıkartma uygulanıyor. İlk üçte birlik dilimi alma yaklaşımı, 1–2 tercihi olan kullanıcıda boş sonuç üretebiliyor. Prototip kullanıcı tablosunda düz metin şifre tanımı var.

aimovieai'de Jaccard yaklaşımı Go'da ağırlıklı olarak yeniden uygulandı. İlk sürümde film verisi yerel kataloğa alınmıştı; TMDB geçişiyle gömülü JSON veri seti ve PostgreSQL `movies` bağımlılığı kaldırıldı. MovieDude kullanıcı/şifre verileri hiçbir zaman taşınmadı.

## movie-list üzerinden TMDB entegrasyonu

`movie-list/internal/tmdb/client.go`, `internal/service/movie_service.go` ve `cmd/server/main.go` incelendi. Proje `/search/movie` için backend'de `TMDB_BASE_URL` ve `TMDB_TOKEN` ile Bearer kimlik doğrulaması kullanıyor. Tüm kataloğu bir kerede indiren bir akış görülmedi; sorguya göre TMDB araması yapılıyor.

Aynı yaklaşım aimovieai'ye uyarlanıp keşif, detay, credits/keywords ve öneri uçlarıyla genişletildi. Context, zaman aşımı, yanıt boyutu sınırı, 10 dakika/256 yanıtlık bellek önbelleği ve hata durumları eklendi. Anahtar frontend'e taşınmadı. Kimlik bilgileri yalnız yerel `.env` üzerinden sağlanır; kaynak belgeleri ve testler gerçek token içermez.

PostgreSQL hesap/oturum tabloları yanında `activities` üzerinden TMDB kimliği, kişisel izleme/beğeni ve puanları tutar. Şema geçişi mevcut tercihleri koruyarak eski katalog tablosunu kaldırır. Öneri motoru TMDB'den istek sırasında topladığı sınırlı adayları sıralar; tüm katalog indirilmez.

Kaynak projeler değiştirilmedi. Önceki incelemeye ait MIT lisans metinleri `third_party/` dizininde korundu. TMDB API kullanımına ilişkin resmî atıf arayüzün alt bilgisinde bulunur.

## Tema tabanlı öneri iyileştirmesi

Tanınmayan istekten sınırsız popüler keşfe geçme kaldırıldı. Tema/hikâye/ruh hali çıkarımı ve adayların gerçek özet/etiketleri üzerinden ikinci değerlendirme eklendi. Kaynak metinde olmayan model gerekçeleri ve aday dışı kimlikler reddedilir. Başarı/azim regresyonu sentetik verilerle doğrulandı. Kullanıcıya netleştirme sorusu ve yorumlanan tema gösterilir; katalog tür filtresi öneri tür filtresinden ayrıldı.
