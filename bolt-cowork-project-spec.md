# Bolt Cowork — Proje Tanım Dokümanı

**Proje Adı:** Bolt Cowork
**Birincil Dil:** Go 1.25+
**Ek Diller:** Shell (otomasyon), TypeScript (GUI)
**Tür:** CLI tabanlı yerel dosya ajan platformu
**İlham Kaynağı:** Claude Cowork (Anthropic)
**Geliştirme Modeli:** İnsan yönlendirmeli, AI destekli geliştirme (Claude Code + OpenAI Codex)
**Lisans:** Açık kaynak (lisans türü belirlenecek)

---

## 1. Vizyon

Bolt Cowork, kullanıcının bilgisayarındaki dosyalara erişen, doğal dil komutlarıyla görev alan ve bu görevleri bir LLM (Large Language Model — Büyük Dil Modeli) aracılığıyla çözen açık kaynaklı bir ajan platformudur.

Claude Cowork'ün temel felsefesini — "sadece cevap verme, işi yap" — alıp Go dilinin güçlü yanlarıyla (concurrency/eşzamanlılık, tek binary derleme, hızlı dosya işlemleri) birleştirir. İlerleyen versiyonlarda Shell (otomasyon) ve TypeScript (kullanıcı arayüzü) ile zenginleştirilir.

---

## 2. Terminoloji — Geliştirme Araçları vs Çalışma Zamanı

Bu projede iki farklı bağlamda AI kullanılır. Karışmaması için:

### Geliştirme Araçları (Development Tools — Geliştirme Araçları)
Bolt Cowork'ün **kodunu yazmak** için kullanılır. Son ürünün parçası değildir.

| Araç | Rol | Ne Zaman Kullanılır |
|------|-----|-------------------|
| **Claude Code** | Birincil geliştirici | Bolt Cowork'ün Go/TS/Shell kodunu yazar, test eder, refactor yapar |
| **OpenAI Codex** | Code reviewer (kod gözden geçirici) | Claude Code'un yazdığı kodu inceler, alternatif önerir |
| **Sen** | Ürün yöneticisi + mimar | Karar verir, onaylar, yönlendirir |

### Çalışma Zamanı Provider'ları (Runtime Providers — Çalışma Zamanı Sağlayıcıları)
Bolt Cowork'ün **kendi beyni** olarak çalışır. Son kullanıcı bunlarla etkileşir.

| Provider | Rol | Ne Zaman Çalışır |
|----------|-----|-----------------|
| **OpenAI API** (GPT modelleri) | LLM sağlayıcı | Kullanıcı Bolt Cowork'e görev verdiğinde |
| **Anthropic API** (Claude modelleri) | LLM sağlayıcı | Kullanıcı Bolt Cowork'e görev verdiğinde |
| **Kendi LLM'iniz** (v0.5) | LLM sağlayıcı | Kullanıcı Bolt Cowork'e görev verdiğinde |

### Akış Şeması

```
┌─────────────────────────────────────────────────────────┐
│  GELİŞTİRME ZAMANI (Development Time)                   │
│                                                          │
│  Sen ──▶ Claude Code / Codex ──▶ Bolt Cowork'ün kodunu   │
│                                   yazar                  │
│                                                          │
│  Bu araçlar son ürünün parçası DEĞİLDİR.                 │
└─────────────────────────────────────────────────────────┘

                        ▼ derleme (build) ▼

┌─────────────────────────────────────────────────────────┐
│  ÇALIŞMA ZAMANI (Runtime)                                │
│                                                          │
│  Son Kullanıcı ──▶ Bolt Cowork ──▶ OpenAI / Anthropic / │
│                                    Kendi LLM'iniz        │
│                                         │                │
│                                         ▼                │
│                                    Görevi yapar           │
│                                    (dosya düzenle,        │
│                                     özetle, vb.)         │
│                                                          │
│  Kullanıcı config'den istediği provider'ı seçer.         │
│  Claude Code / Codex ile hiçbir bağlantı yoktur.         │
└─────────────────────────────────────────────────────────┘
```

---

## 3. Dil Stratejisi

Her dil projeye belirli bir aşamada ve belirli bir gerekçeyle katılır:

| Dil | Giriş Zamanı | Kullanım Alanı |
|-----|-------------|----------------|
| **Go** | v0.1'den itibaren | Çekirdek ajan, CLI, MCP client, skill sistemi, performans kritik işlemler — projenin omurgası |
| **Shell** | v0.1'den itibaren (minimal), v0.4'te genişler | Build/test otomasyonu, MCP sunucu başlatma, CI/CD pipeline, ortam hazırlama scriptleri |
| **TypeScript** | v0.6 | Web tabanlı GUI: React frontend + Go backend API. Alternatif: Electron ile masaüstü uygulaması |

**Prensip:** Yeni bir dil eklemek ancak Go'nun tek başına verimli çözemediği bir problem ortaya çıktığında yapılır. Erken optimizasyondan kaçınılır.

---

## 4. Temel Özellikler (Versiyon Planı)

### v0.1 — Temel Ajan *(Go + minimal Shell)*
- Kullanıcının belirlediği bir klasöre erişim (sandbox/korumalı alan mantığı)
- Doğal dil ile görev tanımlama
- LLM Provider Interface (Sağlayıcı Arayüzü) ile değiştirilebilir model desteği
- Fallback Chain (Yedek Zinciri): model limiti dolunca otomatik olarak bir sonraki modele geçiş
- İlk provider'lar: OpenAI API + Anthropic API
- Agent Loop (Ajan Döngüsü): kullanıcı komutu → plan oluştur → kullanıcı onayı → çalıştır → sonuç raporla
- Temel dosya işlemleri: okuma, yazma, taşıma, silme, yeniden adlandırma, içerik analizi
- Shell: `build.sh`, `test.sh` gibi temel otomasyon scriptleri

### v0.2 — Skill Sistemi / Beceri Sistemi *(Go)*
- `~/.bolt-cowork/skills/` ve `./bolt-skills/` klasörlerinden SKILL.md dosyalarını okuma
- YAML frontmatter ile skill metadata (beceri üst verisi) tanımlama
- Skill'lerin otomatik tetiklenmesi (açıklamaya göre) veya manuel çağrılması (`/skill-adı`)
- Skill içeriğinin LLM prompt'una context (bağlam) olarak enjekte edilmesi
- Varsayılan skill'ler: dosya düzenleyici, özetleyici, kod analizi

### v0.3 — MCP Client / Model Bağlam Protokolü İstemcisi *(Go)*
- JSON-RPC 2.0 tabanlı MCP protokolünü Go ile implemente etme
- stdio transport (standart giriş/çıkış taşıma) desteği
- HTTP transport desteği
- Konfigürasyon dosyasından MCP sunucu tanımları okuma (`~/.bolt-cowork/mcp.json`)
- İlk desteklenen sunucular: filesystem (dosya sistemi), web search (web araması)

### v0.4 — Sub-agent Coordination / Alt Ajan Koordinasyonu *(Go + Shell)*
- Karmaşık görevleri parçalara ayırma (task decomposition)
- Go goroutine'leri ile paralel görev çalıştırma
- Alt ajanlar arası bağımlılık yönetimi (dependency management)
- İlerleme raporlama ve hata yönetimi
- Shell: MCP sunucu yaşam döngüsü yönetimi, ortam hazırlama scriptleri

### v0.5 — Kendi LLM Provider'ı *(Go + Shell)*
- Python + FastAPI ile sarmalanmış özel eğitimli modeli destekleme
- HTTP tabanlı custom provider implementasyonu
- Go ile performans optimizasyonları:
  - Büyük dosya okuma/parse etme (>100MB) — `io.Reader` stream yapısı ile
  - Token sayma ve bölme (tokenization) — Go kütüphaneleri ile
- Model performans karşılaştırması (benchmark) aracı
- Shell: model servis başlatma/durdurma, sağlık kontrolü scriptleri

### v0.6 — GUI / Kullanıcı Arayüzü *(Go + TypeScript)*
- **Birincil seçenek:** Web UI — Go backend API + React/TypeScript frontend
- **Alternatif seçenek:** Electron masaüstü uygulaması (TypeScript frontend + Go backend)
- Gerçek zamanlı görev izleme (WebSocket)
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

| # | Aşama | Kullanıcıya Gösterilen | Seçenekler |
|---|-------|----------------------|------------|
| 1 | Skill eşleştirme | "Bu görev için şu skill'leri kullanmayı planlıyorum: [liste]" | ✅ Onayla / ❌ Reddet / ✏️ Değiştir |
| 2 | Plan oluşturma | "Şu adımları takip edeceğim: [adım listesi]" | ✅ Onayla / ❌ Reddet / ✏️ Revize et |
| 3 | Her çalıştırma adımı | "Şimdi şunu yapacağım: [dosya X'i taşı]" | ✅ Devam / ⏭️ Tümünü onayla / ❌ Durdur |
| 4 | Sonuç | "Görev tamamlandı. Yapılanlar: [özet]" | ✅ Kabul / ↩️ Geri al |

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
- **İnsan (Sen):** Ürün yöneticisi + mimar + onaylayıcı. Neyin yapılacağına, önceliklere, mimari kararlara karar verir. Her çıktıyı inceler ve onaylar.
- **Claude Code:** Birincil geliştirici. Kodun ~%80-90'ını yazar. Ama hiçbir şeyi onaysız commit etmez.
- **OpenAI Codex:** İkincil geliştirici / code reviewer (kod gözden geçirici).

### 7.2 Geliştirme Döngüsü (Detaylı)

```
 ┌─────────────────────────────────────────────────┐
 │  AŞAMA 1: FİKİR (Sen)                           │
 │  Yeni özellik veya değişiklik tanımla            │
 │  "v0.1 için sandbox modülünü yapalım"            │
 │  ☑ SEN karar verirsin                            │
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
 ┌─────────────────────────────────────────────────┐
 │  AŞAMA 5: REVIEW (OpenAI Codex)                  │
 │  Codex aynı kodu farklı perspektiften inceler     │
 │  Alternatif yaklaşımlar ve sorunları raporlar     │
 │  ☑ SEN Codex'in önerilerini değerlendirirsin      │
 └──────────────────┬──────────────────────────────┘
                    ▼
 ┌─────────────────────────────────────────────────┐
 │  AŞAMA 6: BİRLEŞTİRME (Sen + Claude Code)       │
 │  Son kararları sen verirsin                       │
 │  Claude Code commit ve PR oluşturur               │
 │  ☑ SEN merge onayı verirsin                       │
 └─────────────────────────────────────────────────┘
```

### 7.3 Önemli Prensip

Claude Code ve Codex birer araçtır — mimari kararlar, önceliklendirme ve ürün vizyonu her zaman insana aittir. "Nasıl" sorusunu ajanlar cevaplar, "Ne" ve "Neden" sorularını sen cevaplarsın.

---

## 8. Provider Konfigürasyonu

### ~/.bolt-cowork/config.yaml
```yaml
default_provider: anthropic

providers:
  anthropic:
    api_key: ${ANTHROPIC_API_KEY}
    models:
      - claude-opus-4-6          # Birincil — en güçlü
      - claude-sonnet-4-6        # Yedek — hızlı ve ekonomik

  openai:
    api_key: ${OPENAI_API_KEY}
    models:
      - gpt-4o                   # Birincil
      - gpt-4o-mini              # Yedek — düşük maliyet

  custom:                        # v0.5'te aktif olur
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
    - ./workspace              # Kullanıcı burayı kendi belirler
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
  servers: []  # v0.3'te doldurulacak

approval_mode: full  # full | plan-only | dangerous-only | none
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
make test           # Tüm testleri çalıştır
make lint           # Tüm diller için lint
make dev-web        # Web frontend geliştirme sunucusu (v0.6+)

# Doğrudan çalıştırma
./bolt-cowork --dir ./workspace "Bu klasördeki PDF dosyalarını özetle"
./bolt-cowork --dir ./workspace --approval full "Dosyaları türlerine göre ayır"
./bolt-cowork --provider openai --dir ./workspace "README.md oluştur"
```

---

## 10. Bağımlılıklar (Planlanan)

### Go (v0.1+)
| Paket | Amaç |
|-------|-------|
| `github.com/spf13/cobra` | CLI framework (komut satırı çatısı) |
| `github.com/spf13/viper` | Konfigürasyon yönetimi |
| `github.com/sashabaranov/go-openai` | OpenAI API client |
| `github.com/anthropics/anthropic-sdk-go` | Anthropic API client |
| `gopkg.in/yaml.v3` | YAML parse (SKILL.md frontmatter) |

### TypeScript (v0.6+)
| Paket | Amaç |
|-------|-------|
| `react` | UI framework |
| `typescript` | Tip güvenliği |
| `tailwindcss` | Stil (styling) |

### Shell
| Araç | Amaç |
|------|-------|
| `shellcheck` | Lint |
| `make` | Build otomasyonu |

---

## 11. Riskler ve Açık Sorular

| # | Konu | Durum | Çözüm Planı |
|---|------|-------|-------------|
| 1 | GUI tercihi: Web vs Electron vs TUI | v0.6'da karar verilecek | v0.5 sonrasında değerlendir |
| 2 | Kendi LLM'in boyutu ve kapasitesi | Kursa bağlı | v0.5'te netleşecek |
| 3 | MCP Go kütüphanesi olgunluğu | Araştırılacak | Gerekirse kendi implementasyon |
| 4 | Token maliyeti yönetimi | Fallback chain ile azaltılacak | Kullanım limiti + maliyet raporlama |
| 5 | Güvenlik: sandbox bypass riski | v0.1'de temel | Her versiyonda güçlendirilecek |
| 6 | Go performans yeterliliği (büyük dosyalar) | Beklenti: yeterli | Darboğaz çıkarsa profiling ile optimize et |

---

## 12. Başarı Kriterleri

### v0.1 için "Bitti" tanımı:
- [ ] `bolt-cowork --dir ./workspace "Bu klasördeki dosyaları listele"` çalışıyor
- [ ] `bolt-cowork --dir ./workspace "README.md dosyasını özetle"` çalışıyor
- [ ] `bolt-cowork --dir ./workspace "Dosyaları türlerine göre klasörlere ayır"` çalışıyor
- [ ] `--provider openai` ve `--provider anthropic` arasında geçiş yapılabiliyor
- [ ] Fallback chain çalışıyor (birincil model hata verince ikinciye geçiyor)
- [ ] Sandbox dışına erişim engelleniyor
- [ ] Her adımda kullanıcı onayı soruluyor (--approval full)
- [ ] "Tümünü onayla" seçeneği çalışıyor
- [ ] Temel hata mesajları anlaşılır

---

*Bu doküman yaşayan bir belgedir. Her versiyon geçişinde güncellenecektir.*
*Son güncelleme: 24 Mart 2026*
