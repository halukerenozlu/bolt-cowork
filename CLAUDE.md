# Bolt Cowork — Claude Code Proje Hafızası

**Tür:** CLI tabanlı yerel dosya ajan platformu
**Birincil Dil:** Go 1.25+ | **Ek:** Shell (otomasyon), TypeScript (GUI, v0.6+)
**Güncel Versiyon:** v0.1.8
**Detaylı Spec:** `bolt-cowork-project-spec.md`

---

## Terminoloji — Karıştırma!

Bu projede AI iki farklı bağlamda kullanılır:

| Bağlam | Ne İçin | Örnekler |
|--------|---------|----------|
| **Geliştirme Araçları (Development Tools)** | Bolt Cowork'ün **kodunu yazmak** için. Son ürünün parçası DEĞİLDİR. | Claude Code, OpenAI Codex, Gemini CLI |
| **Çalışma Zamanı Provider'ları (Runtime Providers)** | Bolt Cowork'ün **kendi beyni**. Son kullanıcı bunlarla etkileşir. | OpenAI API, Anthropic API, Kendi LLM |

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
├── cmd/bolt-cowork/main.go      # Giriş noktası (entry point)
├── internal/
│   ├── agent/                   # Ajan döngüsü, planlama, çalıştırma
│   ├── provider/                # LLM provider'lar + fallback chain
│   ├── skill/                   # Skill yükleme, eşleştirme, kayıt
│   ├── mcp/                     # MCP client, transport, kayıt
│   ├── sandbox/                 # Dosya erişim kısıtlama
│   └── config/                  # Yapılandırma yönetimi
├── pkg/types/                   # Paylaşılan tip tanımları
├── testdata/                    # ⛔ Testler SADECE burada çalışır
│   ├── sample-dir/              # Sahte kullanıcı klasörü
│   └── fixtures/                # Sabit test verileri
├── scripts/                     # build.sh, test.sh, lint.sh
├── skills/                      # Varsayılan SKILL.md dosyaları
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

type Skill struct {
    Name, Description, Content string
    AutoTrigger                bool
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

| # | Aşama | Seçenekler |
|---|-------|------------|
| 1 | Skill eşleştirme | Onayla / Reddet / Değiştir |
| 2 | Plan oluşturma | Onayla / Reddet / Revize et |
| 3 | Her çalıştırma adımı | Devam / Tümünü onayla / Durdur |
| 4 | Sonuç | Kabul / Geri al |

**Hız Modları:**
- `--approval full` — her adımda dur (varsayılan)
- `--approval plan-only` — sadece planlama aşamasında dur
- `--approval dangerous-only` — sadece silme/üzerine yazma gibi işlemlerde dur
- `--approval none` — tam otomatik

---

## Kodlama Standartları

### Go
- Go 1.25+ kullan
- Hata yönetimi: `fmt.Errorf("context: %w", err)` ile wrap
- Testler table-driven (tablo güdümlü) yaz
- Yorumlar İngilizce
- `golangci-lint` ile lint kontrolü
- Package adları kısa ve açıklayıcı

### Shell
- Bash 5+, `#!/usr/bin/env bash` ile başla
- `set -euo pipefail` her scriptin başında
- ShellCheck ile lint kontrolü

### TypeScript (v0.6+)
- React 19+ ve TypeScript 5+
- ESLint + Prettier
- Fonksiyonel component'ler (class component yok)

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

| Versiyon | Özet | Diller | Durum |
|----------|------|--------|-------|
| v0.1 | Temel ajan: sandbox, LLM provider, fallback chain, dosya işlemleri, onay döngüsü | Go + Shell | ✅ Tamamlandı (v0.1.6) |
| v0.1.7 | Konuşma geçmişi, OpenAI + Gemini provider'ları | Go | ✅ Tamamlandı |
| v0.1.8 | Bug fixes (signal handling, sandbox, fallback, tilde expansion) — Final bug fix release before v0.2 | Go | ✅ Tamamlandı |
| v0.2 | Skill sistemi: SKILL.md okuma, otomatik tetikleme, prompt enjeksiyonu | Go | ← Sıradaki |
| v0.3 | MCP client: JSON-RPC 2.0, stdio/HTTP transport | Go | |
| v0.4 | Alt ajan koordinasyonu: görev parçalama, paralel çalıştırma | Go + Shell | |
| v0.5 | Kendi LLM provider'ı: custom HTTP provider, performans optimizasyonu | Go + Shell | |
| v0.6 | GUI: Web UI (React + Go API) veya Electron | Go + TS | |
