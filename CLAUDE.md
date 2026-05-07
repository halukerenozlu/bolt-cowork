# Bolt Cowork — Claude Code Proje Hafızası

**Tür:** CLI tabanlı yerel dosya ajan platformu
**Birincil Dil:** Go 1.26+ | **Ek:** Shell (otomasyon), TypeScript (GUI, v0.6+)
**Güncel Versiyon:** v0.2.6
**Detaylı Spec:** `bolt-cowork-project-spec.md`

---

## Terminoloji — Karıştırma!

Bu projede AI iki farklı bağlamda kullanılır:

| Bağlam                                               | Ne İçin                                                             | Örnekler                              |
| ---------------------------------------------------- | ------------------------------------------------------------------- | ------------------------------------- |
| **Geliştirme Araçları (Development Tools)**          | Bolt Cowork'ün **kodunu yazmak** için. Son ürünün parçası DEĞİLDİR. | Claude Code, OpenAI Codex, Gemini CLI |
| **Çalışma Zamanı Provider'ları (Runtime Providers)** | Bolt Cowork'ün **kendi beyni**. Son kullanıcı bunlarla etkileşir.   | OpenAI API, Anthropic API, Kendi LLM  |

Claude Code → Birincil geliştirici. Kodu yazar.
OpenAI Codex → Code reviewer. Kodu inceler.
Gemini CLI → Geliştirici + reviewer. Her iki rolde kullanılabilir.
Haluk → Ürün yöneticisi + mimar. Karar verir, onaylar.
Runtime provider → Bolt Cowork çalışırken kullanıcı görevlerini çözer.
Bu ikisi birbirine karıştırılmamalıdır.

---

## Klasör Yapısı

```
bolt-cowork/
├── cmd/bolt-cowork/main.go           # Giriş noktası (entry point)
│   ├── embedded_skills.go            # go:embed direktifi — bundled skills
│   └── skills/                       # Varsayılan SKILL.md dosyaları (binary'ye gömülü)
├── internal/
│   ├── agent/                   # Ajan döngüsü, planlama, çalıştırma
│   ├── provider/                # LLM provider'lar + fallback chain
│   ├── skill/                   # Skill sistemi: loader, matcher, injector (v0.2.4)
│   │   ├── skill.go             # SkillScope, SkillMetadata, Skill struct, SkillStore interface
│   │   ├── frontmatter.go       # parseFrontMatter, descriptionFallback, nameFromPath
│   │   ├── loader.go            # ParseFile, LoadAll (scope assignment), LoadEmbedded, Store
│   │   ├── matcher.go           # Keyword-based matching, stop words filter
│   │   └── injector.go          # BuildSkillContext, InjectSkills (<active_skills> XML)
│   ├── mcp/                     # MCP client, transport, kayıt
│   ├── sandbox/                 # Dosya erişim kısıtlama
│   └── config/                  # Yapılandırma yönetimi
├── pkg/types/                   # Paylaşılan tip tanımları
├── testdata/                    # ⛔ Testler SADECE burada çalışır
│   ├── sample-dir/              # Sahte kullanıcı klasörü
│   └── fixtures/                # Sabit test verileri
├── scripts/                     # build.sh, test.sh, lint.sh
├── web/                         # v0.6'da eklenir (React + TS)
├── go.mod / go.sum
└── Makefile
```

---

## Temel Interface'ler

```go
type LLMProvider interface {
    Chat(ctx context.Context, messages []Message) (string, error)
    StreamChat(ctx context.Context, messages []Message) (<-chan string, error)
    Name() string
    Available() bool
}

type FallbackChain struct {
    providers []LLMProvider
    current   int
}

type SkillMetadata struct {
    Name             string
    Description      string
    Tags             []string
    Priority         int
    AutoTrigger      bool
    RequiresApproval bool
}

type Skill struct {
    Metadata SkillMetadata
    Scope    SkillScope // ScopeBundled | ScopeGlobal | ScopeProject
    Content  string
    FilePath string
}

type SkillStore interface {
    LoadAll(dirs []string) []string
    GetAll() []Skill
    GetByName(name string) (*Skill, error)
}

type MCPClient interface {
    ListTools() ([]Tool, error)
    CallTool(name string, args map[string]any) (Result, error)
    Close() error
}

type Agent struct {
    chain      *FallbackChain
    skills     []Skill
    mcpClients []MCPClient
    sandbox    *Sandbox
}
```

---

## Onay Modeli (Approval Gates)

Ajan döngüsü 4 aşamada kullanıcı onayı bekler:

| #   | Aşama                | Seçenekler                                                     |
| --- | -------------------- | -------------------------------------------------------------- |
| 1   | Skill eşleştirme     | Onayla / Reddet (Modify yok — manuel seçim için `/use <name>`) |
| 2   | Plan oluşturma       | Onayla / Reddet / Revize et                                    |
| 3   | Her çalıştırma adımı | Devam / Tümünü onayla / Durdur                                 |
| 4   | Sonuç                | Kabul / Geri al                                                |

**Hız Modları:**

- `--approval full` — her adımda dur; skill approval **sorar** (varsayılan)
- `--approval plan-only` — sadece plan aşamasında dur; skill approval **sormaz** (otomatik onay)
- `--approval dangerous-only` — sadece silme/üzerine yazma işlemlerinde dur; skill approval **sormaz**
- `--approval none` — tam otomatik; skill approval **sormaz**

---

## Kodlama Standartları

### Go

- Go 1.26+ kullan
- Hata yönetimi: `fmt.Errorf("context: %w", err)` ile wrap
- Testler table-driven (tablo güdümlü) yaz
- Yorumlar İngilizce
- `golangci-lint` ile lint kontrolü
- Package adları kısa ve açıklayıcı
- Skill eşleştirme keyword-based olmalı, LLM-based matching v0.3+ scope (v0.2'de yapılmaz)
- `/use <name>` komutu `SetForceSkills()` ile bir sonraki Run için skill'i force-activate eder (one-shot: Run sonrası otomatik temizlenir)

### Shell

- Bash 5+, `#!/usr/bin/env bash` ile başla
- `set -euo pipefail` her scriptin başında
- ShellCheck ile lint kontrolü

### TypeScript (v0.6+)

- React 19+ ve TypeScript 5+
- ESLint + Prettier
- Fonksiyonel component'ler (class component yok)

---

## Skill Dosya Formatı (v0.2.4)

SKILL.md dosyaları YAML frontmatter + Markdown body formatındadır:

```yaml
---
name: file-organizer
description: Organizes files by type into directories
auto_trigger: true
tags:
  - files
  - automation
priority: 10
requires_approval: false
---
[Markdown body — LLM'e talimatlar]
```

- Frontmatter alanları: `name` (zorunlu), `description` (zorunlu), `auto_trigger` (opsiyonel, default: false), `tags` (opsiyonel), `priority` (opsiyonel, default: 0), `requires_approval` (opsiyonel, default: false)
- Frontmatter yoksa: `name` dosya yolundan türetilir, `description` ilk paragraftan (max 512 karakter)
- CRLF satır sonları otomatik normalize edilir
- **Yükleme sırası (override zinciri):**
  1. Bundled — binary yanındaki `skills/` dizini (yazılımla gelen)
  2. Global — `~/.bolt-cowork/skills/` (kullanıcının kendi skill'leri)
  3. Project-local — `./bolt-skills/` (proje bazlı)
  - Çakışma: aynı `name` varsa **sonraki katman öncekini override eder** (local > global > bundled)
- Eşleştirme: keyword-based (description kelimelerini kullanıcı komutunda arar); `auto_trigger: false` skill'ler otomatik eşleşmez
- Enjeksiyon: planner system message'ına `<active_skills>` XML bloğu olarak
- **ForceSkills (`/use <name>`):**
  - `SetForceSkills()` ile set edilir; bir sonraki `Run()` sonrası **otomatik temizlenir** (one-shot)
  - ForceSkills aktifken `Match()` atlanır, `GetByName()` ile isimden çözümlenir
  - `auto_trigger: false` olan skill'ler de `/use` ile aktive edilebilir
  - Bilinmeyen isim verilirse stderr'e uyarı yazılır ve skip edilir

---

## Test İzolasyon Kuralları ⛔

**İstisna yoktur.**

### Kesin Yasaklar

- Testlerde ASLA `~/Documents`, `~/Desktop`, `~/Downloads` veya herhangi bir gerçek kullanıcı dizini kullanılmaz
- Testlerde ASLA `os.UserHomeDir()` veya `os.Getenv("HOME")` ile gerçek yollara erişilmez
- Testlerde ASLA `/tmp` dışında proje klasörü haricine yazılmaz
- Claude Code geliştirme sırasında ASLA `bolt-cowork/` klasörü dışına çıkmaz

### Zorunlu Kurallar

- Tüm dosya işlem testleri `testdata/` veya `t.TempDir()` içinde çalışır
- `testdata/sample-dir/` sahte kullanıcı klasörü olarak kullanılır
- `testdata/fixtures/` sabit test verileri için kullanılır
- Her test çalıştırmasından önce test verisi oluşturulur, sonra temizlenir (setup/teardown)
- Sandbox modülü de `testdata/` içinde test edilir

---

## Commit Standartları

Conventional Commits formatı, dile göre scope:

- `feat(go/agent): add plan approval step`
- `fix(ts/components): fix button alignment`
- `chore(shell/build): update test script`

---

## Geliştirme Komutları

```bash
make build          # Go binary derle
make install        # $GOPATH/bin'e kur
make test           # Tüm testleri çalıştır
make lint           # Tüm diller için lint
make dev-web        # Web frontend dev sunucusu (v0.6+)

# Doğrudan çalıştırma
./bolt-cowork --dir ./workspace "Bu klasördeki dosyaları listele"
./bolt-cowork --provider openai --dir ./workspace "README.md oluştur"
```

**CI:** GitHub Actions ile her push/PR'da test + vet + build çalışır. Dependabot Go modül güncellemelerini takip eder.

---

## Geliştirme İş Akışı

1. **Fikir** — İnsan yeni özellik/değişiklik tanımlar
2. **Plan** — Claude Code implementasyon planı sunar → İnsan onaylar/revize eder
3. **Kod Yazımı** — Claude Code yazar, her dosya/fonksiyonda durur → İnsan inceler
4. **Test** — Claude Code testleri yazar ve çalıştırır → İnsan kapsamı onaylar
5. **Review** — Codex ve/veya Gemini CLI aynı kodu farklı perspektiften inceler → İnsan değerlendirir
6. **Birleştirme** — İnsan son kararı verir, Claude Code commit/PR oluşturur → İnsan merge onaylar

### Review Zinciri Kuralları

1. **Kodu yazan araç, aynı kodu review edemez.** Claude Code yazdıysa → Codex veya Gemini review eder.
2. **Review sonucu "REQUEST CHANGES" ise** → yazan araç düzeltir, aynı reviewer tekrar inceler.

**Prensip:** Mimari kararlar, önceliklendirme ve ürün vizyonu her zaman insana aittir.

---

## Versiyon Planı

| Versiyon | Özet                                                                                                | Diller     | Durum                  |
| -------- | --------------------------------------------------------------------------------------------------- | ---------- | ---------------------- |
| v0.1     | Temel ajan: sandbox, LLM provider, fallback chain, dosya işlemleri, onay döngüsü                    | Go + Shell | ✅ Tamamlandı (v0.1.6) |
| v0.1.7   | Konuşma geçmişi, OpenAI + Gemini provider'ları                                                      | Go         | ✅ Tamamlandı          |
| v0.1.8   | Bug fixes (signal handling, sandbox, fallback, tilde expansion) — Final bug fix release before v0.2 | Go         | ✅ Tamamlandı          |
| v0.2     | Skill sistemi: SKILL.md okuma, keyword matching, prompt enjeksiyonu, /use aktivasyonu               | Go         | ✅ Tamamlandı          |
| v0.2.4   | SkillMetadata, SkillScope enum, frontmatter parser, system prompt builder, tool registry            | Go         | ✅ Tamamlandı          |
| v0.2.5   | Güvenlik + Kalite Testleri                                                                          | Go         | ✅ Tamamlandı          |
| v0.2.6   | Stabilizasyon + Dokümantasyon                                                                       | Go         | ✅ Tamamlandı          |
| v0.3     | Foundation + MCP client (JSON-RPC 2.0, external tool access) ← next                                 | Go + Shell |
| v0.4     | TUI (charmbracelet/bubbletea terminal interface)                                                    | Go         |
| v0.5     | Sub-agent coordination (parallel tasks via goroutines)                                              | Go + Shell |
| v0.6     | Custom LLM provider (self-trained model support)                                                    | Go + Shell |
| v0.7     | Desktop App — if needed (if TUI is insufficient)                                                    |
