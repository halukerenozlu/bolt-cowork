# Bolt Cowork — Proje Tanım Dokümanı

**Proje Adı:** Bolt Cowork
**Birincil Dil:** Go 1.26+
**Ek Diller:** Shell (otomasyon), TypeScript (Electron) ya da TUI (bubbletea)
**Tür:** CLI tabanlı yerel dosya ajan platformu
**İlham Kaynağı:** Claude Cowork (Anthropic)
**Geliştirme Modeli:** İnsan yönlendirmeli, AI destekli geliştirme (Claude Code + OpenAI Codex + Gemini CLI)
**Lisans:** Açık kaynak (MIT)

---

## 1. Vizyon

Bolt Cowork, kullanıcının bilgisayarındaki dosyalara erişen, doğal dil komutlarıyla görev alan ve bu görevleri bir LLM (Large Language Model — Büyük Dil Modeli) aracılığıyla çözen açık kaynaklı bir ajan platformudur.

Claude Cowork'ün temel felsefesini — "sadece cevap verme, işi yap" — alıp Go dilinin güçlü yanlarıyla (concurrency/eşzamanlılık, tek binary derleme, hızlı dosya işlemleri) birleştirir. İlerleyen versiyonlarda Shell (otomasyon) ve TypeScript (kullanıcı arayüzü) ile zenginleştirilir.

---

## 2. Terminoloji — Geliştirme Araçları vs Çalışma Zamanı

Bu projede iki farklı bağlamda AI kullanılır. Karışmaması için:

### Geliştirme Araçları (Development Tools — Geliştirme Araçları)

Bolt Cowork'ün **kodunu yazmak** için kullanılır. Son ürünün parçası değildir.

| Araç             | Rol                                 | Ne Zaman Kullanılır                                                |
| ---------------- | ----------------------------------- | ------------------------------------------------------------------ |
| **Claude Code**  | Birincil geliştirici                | Bolt Cowork'ün Go/TS/Shell kodunu yazar, test eder, refactor yapar |
| **OpenAI Codex** | Code reviewer (kod gözden geçirici) | Claude Code'un yazdığı kodu inceler, alternatif önerir             |
| **Gemini CLI**   | Geliştirici + reviewer              | Claude Code gibi kod yazar, ayrıca Codex gibi review yapar         |
| **Sen**          | Ürün yöneticisi + mimar             | Karar verir, onaylar, yönlendirir                                  |

### Çalışma Zamanı Provider'ları (Runtime Providers — Çalışma Zamanı Sağlayıcıları)

Bolt Cowork'ün **kendi beyni** olarak çalışır. Son kullanıcı bunlarla etkileşir.

| Provider                             | Rol           | Ne Zaman Çalışır                         |
| ------------------------------------ | ------------- | ---------------------------------------- |
| **OpenAI API** (GPT modelleri)       | LLM sağlayıcı | Kullanıcı Bolt Cowork'e görev verdiğinde |
| **Anthropic API** (Claude modelleri) | LLM sağlayıcı | Kullanıcı Bolt Cowork'e görev verdiğinde |
| **Kendi LLM'iniz** (v0.5)            | LLM sağlayıcı | Kullanıcı Bolt Cowork'e görev verdiğinde |

### Akış Şeması

```
┌──────────────────────────────────────────────────────────────────┐
│  GELİŞTİRME ZAMANI (Development Time)                            │
│                                                                   │
│  Sen ──▶ Claude Code / Codex / Gemini CLI ──▶ Bolt Cowork'ün     │
│                                                kodunu yazar       │
│                                                                   │
│  Bu araçlar son ürünün parçası DEĞİLDİR.                          │
└──────────────────────────────────────────────────────────────────┘

                           ▼ derleme (build) ▼

┌──────────────────────────────────────────────────────────────────┐
│  ÇALIŞMA ZAMANI (Runtime)                                         │
│                                                                   │
│  Son Kullanıcı ──▶ Bolt Cowork ──▶ OpenAI / Anthropic /          │
│                                    Kendi LLM'iniz                 │
│                                         │                         │
│                                         ▼                         │
│                                    Görevi yapar                    │
│                                    (dosya düzenle, özetle, vb.)   │
│                                                                   │
│  Kullanıcı config'den istediği provider'ı seçer.                  │
│  Claude Code / Codex / Gemini ile hiçbir bağlantı yoktur.         │
└──────────────────────────────────────────────────────────────────┘
```

---

## 3. Dil Stratejisi

Her dil projeye belirli bir aşamada ve belirli bir gerekçeyle katılır:

| Dil            | Giriş Zamanı                                  | Kullanım Alanı                                                                                |
| -------------- | --------------------------------------------- | --------------------------------------------------------------------------------------------- |
| **Go 1.26+**   | v0.1'den itibaren                             | Çekirdek ajan, CLI, MCP client, skill sistemi, performans kritik işlemler — projenin omurgası |
| **Shell**      | v0.1'den itibaren (minimal), v0.4'te genişler | Build/test otomasyonu, MCP sunucu başlatma, CI/CD pipeline, ortam hazırlama scriptleri        |
| **TypeScript** | v0.6                                          | Electron ile masaüstü uygulaması ya da bubbletea ile TUI (terminal kullanıcı arayüzü)         |

**Prensip:** Yeni bir dil eklemek ancak Go'nun tek başına verimli çözemediği bir problem ortaya çıktığında yapılır. Erken optimizasyondan kaçınılır.

---

## 4. Temel Özellikler (Versiyon Planı)

### v0.1 — Temel Ajan _(Go + minimal Shell)_

- Kullanıcının belirlediği bir klasöre erişim (sandbox/korumalı alan mantığı)
- Doğal dil ile görev tanımlama
- LLM Provider Interface (Sağlayıcı Arayüzü) ile değiştirilebilir model desteği
- Fallback Chain (Yedek Zinciri): model limiti dolunca otomatik olarak bir sonraki modele geçiş
- İlk provider'lar: OpenAI API + Anthropic API
- Agent Loop (Ajan Döngüsü): kullanıcı komutu → plan oluştur → kullanıcı onayı → çalıştır → sonuç raporla
- Temel dosya işlemleri: okuma, yazma, taşıma, silme, yeniden adlandırma, içerik analizi
- Shell: `build.sh`, `test.sh` gibi temel otomasyon scriptleri

**Durum:** ✅ Tamamlandı (v0.1.0 → v0.1.6)

#### v0.1.6 Öne Çıkanlar

- Readline entegrasyonu (tab completion, komut geçmişi)
- /config, /dir komutları
- Plan revision feedback (RevisionPrompter, max 3 revizyon)

### v0.1.7 — Konuşma Geçmişi + Yeni Provider'lar _(Go)_ ✅

- REPL konuşma geçmişi (multi-turn context)
- OpenAI API provider implementasyonu
- Google Gemini API provider implementasyonu

### v0.1.8 — Bug Fixes _(Go)_ ✅

- Ctrl+C sinyal yönetimi (signal canceller, her iki REPL path)
- dangerous-only modda yazma onayı düzeltmesi (isDangerous)
- `..hidden` sandbox bypass düzeltmesi
- Provider fallback 401/403 desteği
- Delete intent recursive belirsizliği giderildi
- Meta sorularda konuşma hafızası desteği (boş steps)
- Config'de tilde (~) expansion desteği

### v0.2 — Skill Sistemi / Beceri Sistemi _(Go)_ ✅

- `~/.bolt-cowork/skills/` ve `./bolt-skills/` klasörlerinden SKILL.md dosyalarını okuma
- YAML frontmatter ile skill metadata (beceri üst verisi) tanımlama
- Skill'lerin otomatik tetiklenmesi (açıklamaya göre) veya manuel çağrılması (`/skill-adı`)
- Skill içeriğinin LLM prompt'una context (bağlam) olarak enjekte edilmesi
- Varsayılan skill'ler: file-organizer, summarizer

#### Skill Dosya Formatı

YAML frontmatter + Markdown body:

```yaml
---
name: file-organizer
description: Organizes files by type into directories
auto_trigger: true
---
[Markdown body with instructions for LLM]
```

Frontmatter alanları (minimal): `name`, `description`, `auto_trigger`

#### Yükleme Stratejisi

**Eager loading** — uygulama başlarken tüm SKILL.md dosyaları okunur, parse edilir, bellekte tutulur.

#### Dizinler (Yükleme Sırası)

1. **Bundled** — binary yanındaki `skills/` dizini (yazılımla gelen varsayılan skill'ler)
2. **Global** — `~/.bolt-cowork/skills/` (kullanıcının kendi skill'leri)
3. **Project-local** — `./bolt-skills/` (proje bazlı skill'ler)

**Çakışma kuralı:** Aynı `name`'e sahip skill varsa sonraki katman öncekini override eder (project-local > global > bundled).

#### Eşleştirme (Matching)

**Keyword-based matching:**

- Skill `description` alanındaki kelimeler kullanıcı komutunda aranır
- Case-insensitive eşleştirme
- LLM-based matching v0.3+ için düşünülecek (v0.2 scope'unda DEĞİL)

#### Context Injection

- Eşleşen skill'ler planner system message'ına `Active skills:` bloğu olarak enjekte edilir
- Birden fazla skill eşleşirse hepsi enjekte edilir

#### Manuel Çağırma

- REPL'de `/use <name>` komutuyla (örn: `/use file-organizer`)
- Bir sonraki komut için aktive edilir; komut sonrası otomatik temizlenir (one-shot)
- `auto_trigger: false` olan skill'ler de bu yöntemle aktive edilebilir
- Tab completion desteği (`/use` komutu için)

#### Varsayılan Skill'ler

`skills/` dizininde varsayılan olarak gelir:

- `file-organizer` — dosyaları türüne göre dizinlere organize eder
- `summarizer` — dosya/dizin içeriğini özetler

#### Modül Planı (`internal/skill/`)

| Dosya         | Sorumluluk                                                      |
| ------------- | --------------------------------------------------------------- |
| `skill.go`    | `Skill` struct, `SkillStore` interface                          |
| `loader.go`   | SKILL.md parse (YAML frontmatter + Markdown body), dizin tarama |
| `matcher.go`  | Keyword-based matching, kullanıcı komutu → skill eşleştirme     |
| `injector.go` | Eşleşen skill'leri planner prompt'una enjekte etme              |
| `*_test.go`   | Her dosya için table-driven testler                             |

### v0.2.x Yol Haritası (v0.3 MCP Öncesi İyileştirmeler)

> Bu plan Claude, Codex ve Gemini tarafından ortaklaşa oluşturulmuştur.

#### v0.2.1 — Standardizasyon

- [x] Skill doküman hizalama: approval stage seçeneklerini Approve/Reject olarak netleştir, Modify eklenmeyecek (manuel kontrol /use ile sağlanıyor)
- [x] Subcommand hiyerarşisi: /config, /skill yazıldığında alt komutları listele
- [x] CI/CD: .github/workflows/ci.yml (go test, go vet, build)
- [x] .github/ISSUE_TEMPLATE/ (bug report, feature request)
- [x] .github/PULL_REQUEST_TEMPLATE.md
- [x] CONTRIBUTING.md, CODE_OF_CONDUCT.md, LICENSE
- [x] ASCII logo (terminal başlangıç ekranı)
- [x] Deterministic /init komutu (.cowork/ yapısı, LLM olmadan)

#### v0.2.2 — UX/Cila

- [x] ASCII uyumlu spinner ve renkli log çıktıları (Windows ASCII kısıtlamasına uygun: [= ], [== ], |, /, -, \)
- [x] /mode plan ve /mode build kısayol komutları (mevcut approval mode'ların UX dostu kısayolları)
- [ ] README demo animasyonu (VHS veya asciinema ile terminal kaydı)

#### v0.2.3 — Güvenli Genişleme

- [x] Gerçek çalışma dizini desteği (/dir komutu ile) — Sandbox kuralları korunur, otomatik testler gerçek dizinlere dokunmaz
- [x] Context trimming: uzun konuşmalarda token limiti yaklaşınca özetleme mekanizması (son 20 mesaj / 32K karakter)
- [x] Global skill dizini (~/.bolt-cowork/skills/) stabilizasyonu

#### v0.2.4 — Stabilizasyon ✅

- [x] Kapsamlı manuel test (tüm v0.2.x özellikleri)
- [x] v0.3 MCP client için interface hazırlığı (internal refactoring)
- [x] Final doküman güncellemesi
- [x] Codex + Gemini cross-review, bug fix

#### v0.2.5 — Güvenlik + Kalite Testleri ✅

- [x] Secret redaction tests: Redactor struct, dedup, substring replacement (8 tests)
- [x] Protected path tests: read/write/delete denied, traversal and symlink blocked (7 tests)
- [x] Permission reason tests: delete, overwrite, outside sandbox, safe actions, format (5 tests)
- [x] Agent e2e scenario tests: simple create, read+write, dangerous approval/rejection, multi-step, invalid action, skill injection (7 tests)
- [x] Skill parser edge case tests: unicode, large body, multiple delimiters, whitespace, empty file, frontmatter-only, tabs, duplicate keys (8 tests)
- [x] MCP config validation tests: valid full/minimal, missing name/URL, invalid transport, duplicate name, empty list, unknown fields, invalid value type (9 tests)
- [x] .ssh/_, .gnupg/_, .config/bolt-cowork/\* added to protected paths

#### v0.2.6 — Stabilizasyon + Dokümantasyon ✅

- [x] Protected path case-insensitive matching on Windows (F-005)
- [x] NTFS Alternate Data Stream blocking on Windows (F-014)
- [x] `isReservedFilename`: Windows reserved device names blocked (CON, PRN, AUX, NUL, COM1-9, LPT1-9)
- [x] `maxWriteContentBytes`: 1 MB write size limit
- [x] Plan revision feedback prompt visible (F-012)
- [x] `/dir` resolves relative paths, tilde expansion, filepath.Clean normalization (F-008)
- [x] `--dir /nonexistent` exits with error (F-001)
- [x] Error messages: lowercase start, no trailing periods
- [x] Startup sequence: banner → status → warnings → help hint
- [x] Banner reverted to original Unicode BOLT logo
- [x] Go 1.25 → 1.26
- [x] Removed unused `colorRed`, `colorCyan`, `readREPLLine` functions
- [x] VHS demo tape (`demo.tape`) added
- [x] README, CHANGELOG, CLAUDE.md, AGENTS.md, GEMINI.md updated

#### Ertelenen Maddeler

| Madde                             | Erteleme Nedeni                                   | Hedef Versiyon |
| --------------------------------- | ------------------------------------------------- | -------------- |
| Skill registry/install (internet) | Güvenlik modeli gerektirir, MCP önce tamamlanmalı | v0.4+          |
| TUI framework (bubbletea)         | v0.6 hedefi olarak belirlendi                     | v0.6           |
| Kurulum sihirbazı (MSI/Homebrew)  | Ürün henüz CLI core aşamasında                    | v0.5+          |
| Tanıtım sitesi (EN/TR)            | Dış kullanıcı hedeflenince                        | v0.4+          |

---

### v0.3 — MCP Client / Model Bağlam Protokolü İstemcisi _(Go)_ ← Sıradaki

- JSON-RPC 2.0 tabanlı MCP protokolünü Go ile implemente etme
- stdio transport (standart giriş/çıkış taşıma) desteği
- HTTP transport desteği
- Konfigürasyon dosyasından MCP sunucu tanımları okuma (`~/.bolt-cowork/mcp.json`)
- İlk desteklenen sunucular: filesystem (dosya sistemi), web search (web araması)

### v0.4 — Sub-agent Coordination / Alt Ajan Koordinasyonu _(Go + Shell)_

- Karmaşık görevleri parçalara ayırma (task decomposition)
- Go goroutine'leri ile paralel görev çalıştırma
- Alt ajanlar arası bağımlılık yönetimi (dependency management)
- İlerleme raporlama ve hata yönetimi
- Shell: MCP sunucu yaşam döngüsü yönetimi, ortam hazırlama scriptleri

### v0.5 — Kendi LLM Provider'ı _(Go + Shell)_

- Python + FastAPI ile sarmalanmış özel eğitimli modeli destekleme
- HTTP tabanlı custom provider implementasyonu
- Go ile performans optimizasyonları:
  - Büyük dosya okuma/parse etme (>100MB) — `io.Reader` stream yapısı ile
  - Token sayma ve bölme (tokenization) — Go kütüphaneleri ile
- Model performans karşılaştırması (benchmark) aracı
- Shell: model servis başlatma/durdurma, sağlık kontrolü scriptleri

### v0.6 — TUI + Desktop App _(Go + TypeScript)_

- **Birincil seçenek:** TUI — charmbracelet/bubbletea ile terminal kullanıcı arayüzü
- **Alternatif seçenek:** Electron masaüstü uygulaması (TypeScript frontend + Go backend)
- Gerçek zamanlı görev izleme
- Dosya tarayıcı ve klasör seçici
- Skill ve MCP sunucu yönetim paneli
- Karar v0.5 sonrasında verilecek

---

## 5. Mimari Tasarım

### 5.1 Klasör Yapısı

```
bolt-cowork/
├── cmd/
│   └── bolt-cowork/
│       └── main.go                 # Giriş noktası (entry point)
├── internal/
│   ├── agent/
│   │   ├── agent.go                # Ana ajan döngüsü (agent loop)
│   │   ├── planner.go              # Görev planlama
│   │   └── executor.go             # Görev çalıştırma
│   ├── provider/
│   │   ├── provider.go             # LLM Provider interface tanımı
│   │   ├── openai.go               # OpenAI API provider
│   │   ├── anthropic.go            # Anthropic API provider
│   │   ├── custom.go               # Özel LLM provider (v0.5)
│   │   └── fallback.go             # Fallback chain yönetimi
│   ├── skill/
│   │   ├── loader.go               # Skill dosyalarını okuma ve parse etme
│   │   ├── matcher.go              # Görev-skill eşleştirme
│   │   └── registry.go             # Yüklü skill'leri yönetme
│   ├── mcp/
│   │   ├── client.go               # MCP client implementasyonu
│   │   ├── transport.go            # stdio / HTTP transport
│   │   └── registry.go             # MCP sunucu yönetimi
│   ├── sandbox/
│   │   └── sandbox.go              # Dosya erişim kısıtlama
│   └── config/
│       └── config.go               # Yapılandırma yönetimi
├── pkg/
│   └── types/
│       └── types.go                # Paylaşılan tip tanımları
├── testdata/                       # ⛔ Testler SADECE burada çalışır
│   ├── sample-dir/                 # Sahte kullanıcı klasörü (dosya işlem testleri)
│   │   ├── notes.txt
│   │   ├── report.pdf
│   │   └── photo.jpg
│   ├── fixtures/                   # Sabit test dosyaları (skill parse, config vb.)
│   │   ├── sample-skill.md
│   │   └── sample-config.yaml
│   └── README.md                   # "Bu klasör test amaçlıdır" uyarısı
├── web/                            # v0.6'da eklenir
│   ├── package.json
│   └── src/
│       ├── App.tsx
│       └── components/
├── scripts/                        # Shell scriptler
│   ├── build.sh                    # Derleme otomasyonu
│   ├── test.sh                     # Test çalıştırma
│   ├── lint.sh                     # Lint kontrolü
│   └── mcp-start.sh               # MCP sunucu başlatma (v0.4)
├── skills/                         # Varsayılan skill'ler
│   ├── file-organizer/
│   │   └── SKILL.md
│   └── summarizer/
│       └── SKILL.md
├── go.mod
├── go.sum
├── CLAUDE.md                       # Claude Code proje hafızası
├── AGENTS.md                       # OpenAI Codex proje talimatları
├── Makefile                        # Tüm diller için birleşik build
└── README.md
```

### 5.2 Temel Interface'ler

```go
// LLM Provider — Hangi modelle konuşulacağını soyutlar
type LLMProvider interface {
    Chat(ctx context.Context, messages []Message) (string, error)
    StreamChat(ctx context.Context, messages []Message) (<-chan string, error)
    Name() string
    Available() bool  // Model limiti doldu mu kontrolü
}

// FallbackChain — Model limiti dolunca sıradaki provider'a geçer
type FallbackChain struct {
    providers []LLMProvider  // Öncelik sırasına göre
    current   int
}

// Skill — Yüklenmiş bir beceriyi temsil eder
type Skill struct {
    Name        string
    Description string
    Content     string
    AutoTrigger bool
}

// MCPClient — Bir MCP sunucusuyla iletişimi soyutlar
type MCPClient interface {
    ListTools() ([]Tool, error)
    CallTool(name string, args map[string]any) (Result, error)
    Close() error
}

// Agent — Ana ajan döngüsü
type Agent struct {
    chain      *FallbackChain
    skills     []Skill
    mcpClients []MCPClient
    sandbox    *Sandbox
}
```

### 5.3 Fallback Chain (Yedek Zinciri) Sistemi

```
Kullanıcı komutu geldi
        │
        ▼
┌─ Provider #1: claude-opus-4-6 ──┐
│  Limit doldu mu?                 │
│  ├─ Hayır → Kullan              │
│  └─ Evet  → Sonraki provider    │
└──────────┬──────────────────────┘
           │
           ▼
┌─ Provider #2: claude-sonnet-4-6 ┐
│  Limit doldu mu?                 │
│  ├─ Hayır → Kullan              │
│  └─ Evet  → Sonraki provider    │
└──────────┬──────────────────────┘
           │
           ▼
┌─ Provider #3: gpt-4o ───────────┐
│  Limit doldu mu?                 │
│  ├─ Hayır → Kullan              │
│  └─ Evet  → Hata mesajı         │
└─────────────────────────────────┘

Kullanıcıya her geçişte bildirim yapılır:
"⚠ Opus limiti doldu, Sonnet'e geçiliyor."
```

### 5.4 Ajan Döngüsü (Agent Loop)

```
Kullanıcı komutu
       │
       ▼
┌─ Skill Eşleştirme ─────────┐
│ İlgili skill var mı?        │
│ ☑ KULLANICI ONAYI           │
└──────┬──────────────────────┘
       │
       ▼
┌─ Planlama ──────────────────┐
│ LLM'e gönder:                │
│ - Kullanıcı komutu           │
│ - Skill bağlamı              │
│ - Mevcut araçlar (MCP)       │
│ - Klasör içeriği             │
│                              │
│ LLM plan döndürür            │
│ ☑ KULLANICI ONAYI            │
└──────┬──────────────────────┘
       │
       ▼
┌─ Çalıştırma ────────────────┐
│ Her adımı sırayla çalıştır   │
│ ☑ KULLANICI ONAYI (her adım) │
│ - Dosya işlemleri            │
│ - MCP araç çağrıları        │
│ - Alt ajan görevleri         │
└──────┬──────────────────────┘
       │
       ▼
┌─ Sonuç ─────────────────────┐
│ Sonuç raporu                 │
│ ☑ KULLANICI ONAYI (kabul/red)│
└─────────────────────────────┘
```

---

## 6. Kullanıcı Müdahale Modeli

"Her adımda onay" seçildiği için Bolt Cowork şu onay noktalarında durur:

### 6.1 Onay Noktaları (Approval Gates)

| #   | Aşama                | Kullanıcıya Gösterilen                                        | Seçenekler                                                         |
| --- | -------------------- | ------------------------------------------------------------- | ------------------------------------------------------------------ |
| 1   | Skill eşleştirme     | "Bu görev için şu skill'leri kullanmayı planlıyorum: [liste]" | ✅ Onayla / ❌ Reddet (Değiştir yok — manuel seçim: `/use <name>`) |
| 2   | Plan oluşturma       | "Şu adımları takip edeceğim: [adım listesi]"                  | ✅ Onayla / ❌ Reddet / ✏️ Revize et                               |
| 3   | Her çalıştırma adımı | "Şimdi şunu yapacağım: [dosya X'i taşı]"                      | ✅ Devam / ⏭️ Tümünü onayla / ❌ Durdur                            |
| 4   | Sonuç                | "Görev tamamlandı. Yapılanlar: [özet]"                        | ✅ Kabul / ↩️ Geri al                                              |

### 6.2 Hız Modu (Opsiyonel)

Her adımda onay vermek öğrenme için mükemmel ama zamanla yavaş gelebilir. Bunun için ileride eklenecek modlar:

```bash
# Tam kontrol — her adımda dur (varsayılan)
bolt-cowork --approval full

# Plan onayı — sadece planlama aşamasında dur
bolt-cowork --approval plan-only

# Tehlikeli işlemlerde dur — sadece silme/üzerine yazma gibi işlemlerde
bolt-cowork --approval dangerous-only

# Tam otomatik — hiç durma (deneyimli kullanıcılar için)
bolt-cowork --approval none
```

Başlangıçta `full` modu varsayılan olur. Projeye alıştıkça kendiniz değiştirirsiniz.

---

## 7. Geliştirme İş Akışı — "Her Adımda Onay" Modeli

### 7.1 Roller

- **İnsan (Haluk):** Ürün yöneticisi + mimar + onaylayıcı. Neyin yapılacağına, önceliklere, mimari kararlara karar verir. Her çıktıyı inceler ve onaylar.
- **Claude Code:** Birincil geliştirici. Kodun ~%80-90'ını yazar. Ama hiçbir şeyi onaysız commit etmez.
- **OpenAI Codex:** Code reviewer (kod gözden geçirici).
- **Gemini CLI:** Geliştirici + reviewer. Her iki rolde kullanılabilir.

### 7.2 Geliştirme Döngüsü (Detaylı)

```
 ┌─────────────────────────────────────────────────┐
 │  AŞAMA 1: FİKİR (Sen)                           │
 │  Yeni özellik veya değişiklik tanımla           │
 │  "v0.1 için sandbox modülünü yapalım"           │
 │  ☑ SEN karar verirsin                           │
 └──────────────────┬──────────────────────────────┘
                    ▼
 ┌─────────────────────────────────────────────────┐
 │  AŞAMA 2: PLAN (Claude Code — Plan Modu)         │
 │  Claude Code implementasyon planı sunar           │
 │  "sandbox.go şu interface'leri içerecek..."       │
 │  ☑ SEN planı inceler, onaylar veya revize edersin │
 └──────────────────┬──────────────────────────────┘
                    ▼
 ┌─────────────────────────────────────────────────┐
 │  AŞAMA 3: KOD YAZIMI (Claude Code)               │
 │  Claude Code kodu yazar                           │
 │  Her dosya/fonksiyon tamamlandığında durur         │
 │  ☑ SEN kodu inceler, onaylar veya düzeltme istersin│
 └──────────────────┬──────────────────────────────┘
                    ▼
 ┌─────────────────────────────────────────────────┐
 │  AŞAMA 4: TEST (Claude Code)                     │
 │  Claude Code testleri yazar ve çalıştırır         │
 │  Test sonuçlarını sana gösterir                   │
 │  ☑ SEN test kapsamını ve sonuçlarını onaylarsın   │
 └──────────────────┬──────────────────────────────┘
                    ▼
 ┌─────────────────────────────────────────────────────┐
 │  AŞAMA 5: REVIEW (OpenAI Codex + Gemini CLI)        │
 │  Codex ve/veya Gemini aynı kodu farklı perspektiften │
 │  inceler                                             │
 │  Alternatif yaklaşımlar ve sorunları raporlar        │
 │  ☑ SEN review'ları değerlendirirsin                  │
 └──────────────────┬──────────────────────────────────┘
                    ▼
 ┌─────────────────────────────────────────────────┐
 │  AŞAMA 6: BİRLEŞTİRME (Sen + Claude Code)       │
 │  Son kararları sen verirsin                       │
 │  Claude Code commit ve PR oluşturur               │
 │  ☑ SEN merge onayı verirsin                       │
 └─────────────────────────────────────────────────┘
```

### 7.3 Review Zinciri Kuralları

1. **Kodu yazan araç, aynı kodu review edemez.** Claude Code yazdıysa → Codex veya Gemini review eder.
2. **Review sonucu "REQUEST CHANGES" ise** → yazan araç düzeltir, aynı reviewer tekrar inceler.

### 7.4 Önemli Prensip

Claude Code, Codex ve Gemini birer araçtır — mimari kararlar, önceliklendirme ve ürün vizyonu her zaman insana aittir. "Nasıl" sorusunu ajanlar cevaplar, "Ne" ve "Neden" sorularını sen cevaplarsın.

---

## 8. Provider Konfigürasyonu

### ~/.bolt-cowork/config.yaml

```yaml
default_provider: anthropic

providers:
  anthropic:
    api_key: ${ANTHROPIC_API_KEY}
    models:
      - claude-opus-4-6 # Birincil — en güçlü
      - claude-sonnet-4-6 # Yedek — hızlı ve ekonomik

  openai:
    api_key: ${OPENAI_API_KEY}
    models:
      - gpt-4o # Birincil
      - gpt-4o-mini # Yedek — düşük maliyet

  custom: # v0.5'te aktif olur
    endpoint: http://localhost:8000/chat
    models:
      - bolt-local-v1

# Fallback sırası: yukarıdan aşağıya denenenir.
# Bir model limit/hata verirse sıradaki denenir.
# Kullanıcıya her geçişte bildirim yapılır.

fallback_chain:
  - provider: anthropic
    model: claude-opus-4-6
  - provider: anthropic
    model: claude-sonnet-4-6
  - provider: openai
    model: gpt-4o
  - provider: openai
    model: gpt-4o-mini

sandbox:
  allowed_dirs:
    - ./workspace # Kullanıcı burayı kendi belirler
  denied_patterns:
    - "*.env"
    - "*.key"
    - ".ssh/*"

# ⚠ GELİŞTİRME/TEST SIRASINDA BU AYAR KULLANILMAZ.
# Testler sadece testdata/ ve t.TempDir() içinde çalışır.
# Bu config yalnızca son kullanıcı Bolt Cowork'ü çalıştırdığında geçerlidir.

skills:
  dirs:
    - ~/.bolt-cowork/skills
    - ./bolt-skills

mcp:
  servers: [] # v0.3'te doldurulacak

approval_mode: full # full | plan-only | dangerous-only | none
```

---

## 9. Geliştirme Kuralları

### 9.1 Go Kodlama Standartları

- Go 1.25+ kullan
- Hata yönetiminde `fmt.Errorf("context: %w", err)` ile wrap (sarmalama) yap
- Testleri table-driven (tablo güdümlü) yaz
- Yorumlar İngilizce olsun
- `golangci-lint` ile lint kontrolü yap
- Package (paket) adları kısa ve açıklayıcı olsun

### 9.2 Test İzolasyon Kuralları ⛔

Bu kurallar hem Claude Code'un geliştirme sırasındaki davranışını hem de Bolt Cowork'ün test süitini kapsar. **İstisna yoktur.**

**Kesin Yasaklar:**

- Testlerde ASLA `~/Documents`, `~/Desktop`, `~/Downloads` veya herhangi bir gerçek kullanıcı dizini kullanılmaz
- Testlerde ASLA `os.UserHomeDir()` veya `os.Getenv("HOME")` ile gerçek yollara erişilmez
- Testlerde ASLA `/tmp` dışında proje klasörü haricine yazılmaz
- Claude Code geliştirme sırasında ASLA `bolt-cowork/` klasörü dışına çıkmaz

**Zorunlu Kurallar:**

- Tüm dosya işlem testleri `testdata/` klasöründe veya `t.TempDir()` ile oluşturulan geçici dizinde çalışır
- `t.TempDir()` Go'nun test framework'ünün sağladığı bir fonksiyondur — her test için benzersiz geçici bir klasör oluşturur ve test bitince otomatik siler
- `testdata/sample-dir/` sahte kullanıcı klasörü olarak kullanılır
- `testdata/fixtures/` sabit test verileri (skill dosyaları, config örnekleri vb.) için kullanılır
- Her test çalıştırmasından önce test verisi oluşturulur, sonra temizlenir (setup/teardown)
- Sandbox modülünün kendisi de `testdata/` içinde test edilir — gerçek klasörlere erişimi engellediğini doğrulamak için

**Örnek Test Yapısı:**

```go
func TestSandbox_BlocksOutsideAccess(t *testing.T) {
    // Geçici dizin oluştur — test bitince otomatik silinir
    dir := t.TempDir()

    sb := sandbox.New(dir)

    // İzin verilen dizin içinde çalışmalı
    err := sb.WriteFile(filepath.Join(dir, "test.txt"), []byte("ok"))
    assert(t, err == nil)

    // İzin verilen dizin dışına erişim ENGELLENMELİ
    err = sb.WriteFile("/home/user/Documents/hack.txt", []byte("bad"))
    assert(t, err != nil)  // Hata dönmeli
}
```

### 9.3 TypeScript Kodlama Standartları (v0.6+)

- React 19+ ve TypeScript 5+ kullan
- ESLint + Prettier ile format kontrolü
- Component'ler fonksiyonel olmalı (class component yok)

### 9.4 Shell Script Standartları

- Bash 5+ kullan, `#!/usr/bin/env bash` ile başla
- `set -euo pipefail` her scriptin başında olmalı
- ShellCheck ile lint kontrolü yap

### 9.5 Commit Standartları

- Conventional Commits formatı kullan
- Dile göre scope belirle: `feat(go/agent):`, `fix(ts/components):`, `chore(shell/build):`

### 9.6 Geliştirme Komutları

```bash
# Makefile üzerinden birleşik komutlar
make build          # Go binary derle
make install        # $GOPATH/bin'e kur
make test           # Tüm testleri çalıştır
make lint           # Tüm diller için lint
make dev-web        # Web frontend geliştirme sunucusu (v0.6+)

# Doğrudan çalıştırma
./bolt-cowork --dir ./workspace "Bu klasördeki PDF dosyalarını özetle"
./bolt-cowork --dir ./workspace --approval full "Dosyaları türlerine göre ayır"
./bolt-cowork --provider openai --dir ./workspace "README.md oluştur"
```

**CI/CD:** GitHub Actions ile her push/PR'da test + vet + build çalışır. Dependabot Go modül güncellemelerini takip eder.

---

## 10. Bağımlılıklar (Planlanan)

### Go (v0.1+)

| Paket                                    | Amaç                              |
| ---------------------------------------- | --------------------------------- |
| `github.com/chzyer/readline`             | Readline (tab completion, geçmiş) |
| `gopkg.in/yaml.v3`                       | YAML parse (SKILL.md frontmatter) |
| `github.com/sashabaranov/go-openai`      | OpenAI API client _(v0.1.7)_      |
| `github.com/anthropics/anthropic-sdk-go` | Anthropic API client _(v0.1.7)_   |

### TypeScript (v0.6+)

| Paket         | Amaç           |
| ------------- | -------------- |
| `react`       | UI framework   |
| `typescript`  | Tip güvenliği  |
| `tailwindcss` | Stil (styling) |

### Shell

| Araç         | Amaç             |
| ------------ | ---------------- |
| `shellcheck` | Lint             |
| `make`       | Build otomasyonu |

---

## 11. Riskler ve Açık Sorular

| #   | Konu                                       | Durum                          | Çözüm Planı                                |
| --- | ------------------------------------------ | ------------------------------ | ------------------------------------------ |
| 1   | GUI tercihi: Web vs Electron vs TUI        | v0.6'da karar verilecek        | v0.5 sonrasında değerlendir                |
| 2   | Kendi LLM'in boyutu ve kapasitesi          | Kursa bağlı                    | v0.5'te netleşecek                         |
| 3   | MCP Go kütüphanesi olgunluğu               | Araştırılacak                  | Gerekirse kendi implementasyon             |
| 4   | Token maliyeti yönetimi                    | Fallback chain ile azaltılacak | Kullanım limiti + maliyet raporlama        |
| 5   | Güvenlik: sandbox bypass riski             | v0.1'de temel                  | Her versiyonda güçlendirilecek             |
| 6   | Go performans yeterliliği (büyük dosyalar) | Beklenti: yeterli              | Darboğaz çıkarsa profiling ile optimize et |

---

## 12. Başarı Kriterleri

### v0.1 için "Bitti" tanımı:

- [x] `bolt-cowork --dir ./workspace "Bu klasördeki dosyaları listele"` çalışıyor
- [x] `bolt-cowork --dir ./workspace "README.md dosyasını özetle"` çalışıyor
- [x] `bolt-cowork --dir ./workspace "Dosyaları türlerine göre klasörlere ayır"` çalışıyor
- [x] `--provider openai` ve `--provider anthropic` arasında geçiş yapılabiliyor
- [x] Fallback chain çalışıyor (birincil model hata verince ikinciye geçiyor)
- [x] Sandbox dışına erişim engelleniyor
- [x] Her adımda kullanıcı onayı soruluyor (--approval full)
- [x] "Tümünü onayla" seçeneği çalışıyor
- [x] Temel hata mesajları anlaşılır

---

### v0.1.7 için "Bitti" tanımı:

- [x] REPL konuşma geçmişi çalışıyor (multi-turn context)
- [x] OpenAI API provider implementasyonu çalışıyor
- [x] Google Gemini API provider implementasyonu çalışıyor
- [x] /model komutu provider'lar arası geçiş yapabiliyor
- [x] Fallback chain yeni provider'larla çalışıyor
- [x] Tüm testler geçiyor

---

### v0.1.8 için "Bitti" tanımı:

- [x] Ctrl+C komutu iptal ediyor, REPL'i öldürmüyor
- [x] dangerous-only modda write onay istiyor
- [x] `..hidden` dizinlere sandbox içinden erişilebiliyor
- [x] 401/403 provider fallback'i tetikliyor
- [x] Delete recursive davranışı açık kurallarla tanımlanmış
- [x] Meta sorular konuşma geçmişinden yanıtlanıyor
- [x] Config'deki ~ yolları doğru çözümleniyor
- [x] Tüm testler geçiyor

---

### v0.2 için "Bitti" tanımı: _(Tamamlandı: 25 Nisan 2026)_

- [x] `~/.bolt-cowork/skills/` klasöründen SKILL.md dosyaları okunuyor
- [x] `./bolt-skills/` klasöründen proje bazlı skill'ler okunuyor
- [x] YAML frontmatter (name, description) parse ediliyor
- [x] Skill'ler kullanıcı komutuna göre otomatik tetikleniyor
- [x] `/use <name>` ile manuel çağrılabiliyor (one-shot ForceSkills)
- [x] Skill içeriği LLM prompt'una context olarak enjekte ediliyor (`<active_skills>` XML bloğu)
- [x] Varsayılan skill'ler (file-organizer, summarizer) çalışıyor
- [x] Tüm testler geçiyor

---

_Bu doküman yaşayan bir belgedir. Her versiyon geçişinde güncellenecektir._
_Son güncelleme: 1 Mayıs 2026_
