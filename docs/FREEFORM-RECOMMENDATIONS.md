# Serbest konu ile film önerileri — 2026-10-23

## Bulgu

“Zombi filmi öner” TMDB aramasından önce needs_clarification dönüyordu. `.env` qwen2.5:3b seçmişti; Ollama modelin kurulu olmadığını doğruladı. Hata sessizce yerel kurallara düşüyor, listede olmayan anlaşılır konular belirsiz sayılıyordu.

Önce kurulu llama3.2 denendi. Zombi isteği sonuç verdi ancak başka bir Türkçe isteği yanlış yorumladı; bu ara deneme genel kalite kanıtı sayılmadı. İzinli denemelerde qwen2.5:3b de bazı Türkçe ayrıntıları kaybetti. 7B model 8 GB RAM’de 120 saniyede yüklenemediği için durduruldu. Deneme için indirilen kullanılmayan qwen2.5:3b/7b modelleri son çözümde tutulmaz. Son model yaklaşık 2.5 GB qwen3:4b; think:false ile kısa yanıt kipi kullanılır. Modelin belleğe yüklenmesi doğrulandı. [Model](https://ollama.com/library/qwen3:4b), [thinking kontrolü](https://docs.ollama.com/capabilities/thinking). İlk kapsamlı on alanlı yanıt şeması küçük modelde çelişkili tür/dışlama ve gereksiz belirsizlik işaretleri ürettiği için etkin arama planı üç alana sadeleştirildi.

## Güncel akış

1. Metinli istek için LLM konu kabul listesi olmadan bir JSON arama planı üretir: `story_arc` (isteğin kısa İngilizce anlamı ve koşulları), `keywords` (en fazla üç İngilizce etiket) ve `search_titles` (en fazla iki gerçek film adı ipucu; taşıma katmanı en fazla üçe izin verir). Kullanıcı temaları/türleri önceden seçmek zorunda değildir. Gerçekten anlaşılamayan girdide plan boş olabilir.
2. Yanıt JSON Schema ile yapılandırılır ve şema talimata da eklenir. Şema konu adlarını enum olarak belirlemez. Referans: [Ollama structured outputs](https://docs.ollama.com/capabilities/structured-outputs).
3. TMDB etiketleri tam adla çözülür. Tam etiket yokluğu tek başına aramayı kesmez. Film adı ipuçları search ile gerçek TMDB kimliklerine çözülür; uydurma veya eşleşmeyen başlık sonuç olarak kullanılmaz. Bulunamayan referans başlığı, başka anlam/arama ipucu varsa aramayı kesmez.
4. Gerçek başlık eşleşmeleri ve iki discover sayfası önceliklidir; model başlıklarının ilgili film önerileri havuza sonradan eklenir. Böylece ilk bir başlığın benzerleri daha özel adayları tamamen öne kapatmaz. Yalnız arayüzde seçilen tür/referansla metinsiz istek, model gerektirmeyen benzerlik akışını kullanabilir.
5. Semantik değerlendirmede en fazla sekiz adayın `en-US` detayları dört işçiyle alınır. Arayüzde Türkçe listedeki başlık/özet korunur; İngilizce kanıt özeti ayrı, JSON yanıtına doğrudan çıkmayan alandadır. İngilizce detay alınamaması normal TMDB hata akışını izler.
6. Backend gerçek özetten en fazla üç kısa parça ve en fazla sekiz anahtar kelime çıkarır. LLM en fazla dört güçlü eşleşme için film kimliği, uygunluk ve `evidence_id` seçer. Alıntıyı model yazmaz: backend o filme ait gerçek kaynak parçasını indeksinden alır. Havuz dışı film veya aralık dışı kanıt indeksi reddedilir. Gerekçede varsa modelden gelen ana konuya karşılık gelen gerçek özet/etiket tercih edilir. Kabul edilenler arasında doğrudan ana konu kanıtı varsa, yalnız ikincil ipuçlarına dayananlar ayıklanır; hiçbir doğrudan metin eşleşmesi yoksa anlamsal/parafraz eşleşmeleri yasaklanmaz. Bu dinamik tercih yeni bir izinli konu listesi değildir. Geçerli bir sonuç, yanındaki hatalı model kaydı nedeniyle kaybolmaz.
7. Model erişimi/yorumlama veya değerlendirme başarısızsa API 503 ve frontend yeniden deneme hatası verir. Model arızası kullanıcı belirsizliği gibi gösterilmez; bir konu listesine sessizce düşülmez. Sonuç bulunamazsa popüler filmlerle dolgu yapılmaz.

TMDB tür sözlüğü API kimlik eşlemesidir. Eski yerel tema fonksiyonları etkin metin yorumlamanın kabul kapısı değildir; yerel tür dışlaması ek destek olarak kalır. Confidence alanı eski istemci uyumluluğu için tutulur; üç alanlı planın kabul değeri kalibre edilmiş model olasılığı değildir. Relevance puanı da kalibre edilmiş olasılık değildir.

## Çalışma sınırları

Tek istekte tüm TMDB kataloğu taranmaz; sonuç aday havuzuna ve modelin yorumuna bağlıdır. Konu sözlüğü sınırı olmaması her ifadede kusursuz sonuç garantisi değildir. Semantik model en fazla dört güçlü eşleşme, genel benzerlik akışı en fazla 12 film döndürür. Model aşaması başına 60 saniye, toplam API bütçesi 145 saniye; 4096 bağlam boyutu. Giriş uzunluğu ve aday sayısı gibi kaynak sınırları konu sınırı değildir.

## Testler

`backend/freeform_test.go`: sözlük dışı konuların kabulü; model hatası/belirsizlik ayrımı; tam etiket bulunmadan aday arama; yanlış referans başlığının aramayı kesmemesi; model hatasında sabit konu kurallarına dönülmemesi; Türkçe görünümü koruyarak İngilizce kanıt kullanımı; gerçek/geçersiz kanıt indeksi. Mevcut testlerde uydurma film/alıntı, alakasız aday, izlenenler ve dışlanan tür kontrolleri korunur.

Canlı denemelerin son sonuçları docs/VERIFICATION.md içinde kaydedilir. Önceki llama3.2 zombi denemesi başarılı olsa da diğer konu/ilk şema denemeleri başarısız veya alakasızdı; bunlar son doğrulama yerine geçmez. Görsel/tarayıcı tıklama doğrulaması bu değişiklikte yapılmadı.

## Son canlı sonuç

Çalışan frontend proxy üzerinden Qwen3 ile zombi isteği dört ilgili film, deniz feneri isteği Deniz Feneri (503919) döndürdü. Süreler yaklaşık 65.4 ve 51.3 saniye. Son SQL/race/vet/build kontrolleri ve örneğin tek kişi/iki bekçi gibi anlamsal sınırları [VERIFICATION.md](VERIFICATION.md) içinde yazılıdır. Bu testler geniş model kalite ölçümü değildir.
