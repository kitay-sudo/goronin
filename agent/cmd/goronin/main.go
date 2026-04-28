// goronin — standalone honeypot guard.
//
// Subcommands:
//   install      — interactive wizard, writes config.yml + systemd unit, starts service
//   reconfigure  — re-runs the wizard, restarts the service
//   uninstall    — stop service, remove unit + binary + config + data
//   daemon       — the actual long-running process (called by systemd, not the user)
//   start/stop/restart/status/logs — systemd wrappers
//   unban <ip>   — remove an IP from the GORONIN-BLOCK chain
//   reset        — flush iptables chain and clear persisted blocks
//   version      — print build info
//
// Bare invocation prints usage. The `daemon` mode is invoked by the systemd
// unit and is what actually listens for traps and sends alerts.
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kitay-sudo/goronin/agent/internal/aggregator"
	"github.com/kitay-sudo/goronin/agent/internal/ai"
	"github.com/kitay-sudo/goronin/agent/internal/alerter"
	"github.com/kitay-sudo/goronin/agent/internal/config"
	"github.com/kitay-sudo/goronin/agent/internal/firewall"
	"github.com/kitay-sudo/goronin/agent/internal/storage"
	"github.com/kitay-sudo/goronin/agent/internal/systemd"
	"github.com/kitay-sudo/goronin/agent/internal/telegram"
	"github.com/kitay-sudo/goronin/agent/internal/traps"
	"github.com/kitay-sudo/goronin/agent/internal/watcher"
	"github.com/kitay-sudo/goronin/agent/internal/wizard"
	"github.com/kitay-sudo/goronin/agent/pkg/protocol"
)

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "install":
		runInstall()
	case "reconfigure":
		runReconfigure()
	case "uninstall":
		runUninstall()
	case "daemon":
		runDaemon()
	case "start":
		mustSystemd(systemd.Start)
	case "stop":
		mustSystemd(systemd.Stop)
	case "restart":
		mustSystemd(systemd.Restart)
	case "status":
		_ = systemd.Status() // exit code reflects state
	case "logs":
		follow := len(os.Args) > 2 && (os.Args[2] == "-f" || os.Args[2] == "--follow")
		_ = systemd.Logs(follow)
	case "unban":
		runUnban()
	case "reset":
		runReset()
	case "version", "-v", "--version":
		fmt.Println("goronin", version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`GORONIN — honeypot guard

Использование:
  goronin install              интерактивная установка (требует root)
  goronin reconfigure          перезапустить wizard и перезагрузить сервис
  goronin uninstall            полное удаление (сервис, бинарь, конфиг, данные)
  goronin start                запустить сервис
  goronin stop                 остановить сервис
  goronin restart              перезапустить сервис
  goronin status               статус сервиса
  goronin logs [-f]            показать логи (-f — следить)
  goronin unban <ip>           разблокировать IP вручную
  goronin reset                сбросить все баны и очистить iptables
  goronin version              версия

  goronin daemon               запуск демона (вызывается systemd, не вручную)
`)
}

// ---------- install / reconfigure ----------

func runInstall() {
	mustRoot()

	// Idempotency: if a config already exists we're being re-run on top of
	// an existing install (typical for `curl | sudo bash` doing an update).
	// Don't blow away the wizard's prior answers — tell the user what to do.
	if _, err := os.Stat(config.DefaultPath); err == nil {
		fmt.Println("GORONIN уже установлен (найден", config.DefaultPath+").")
		fmt.Println()
		fmt.Println("  Изменить настройки:   sudo goronin reconfigure")
		fmt.Println("  Полностью удалить:    sudo goronin uninstall")
		fmt.Println()
		fmt.Println("Бинарь обновлён, сервис перезапускаю…")
		if systemd.UnitExists() {
			if err := systemd.Restart(); err != nil {
				fail("restart:", err)
			}
			fmt.Println("✓ Сервис перезапущен с новой версией")
		}
		return
	}

	cfg, err := wizard.Run(os.Stdin, os.Stdout)
	if err != nil {
		fail("install:", err)
	}

	if err := config.Save(config.DefaultPath, cfg); err != nil {
		fail("save config:", err)
	}
	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		fail("create data dir:", err)
	}

	binPath, err := os.Executable()
	if err != nil {
		fail("locate binary:", err)
	}

	fmt.Println()
	fmt.Println("Устанавливаю systemd unit…")
	if err := systemd.Install(binPath); err != nil {
		fail("install systemd unit:", err)
	}
	if err := systemd.Enable(); err != nil {
		fail("enable service:", err)
	}
	if err := systemd.Start(); err != nil {
		fail("start service:", err)
	}

	fmt.Println()
	fmt.Println("✓ GORONIN установлен и запущен")
	fmt.Println()
	fmt.Println("  Управление:  goronin status | logs -f | restart | stop")
	fmt.Println("  Конфиг:     ", config.DefaultPath)
	fmt.Println("  Данные:     ", cfg.DataDir)
	fmt.Println()
}

func runReconfigure() {
	mustRoot()
	cfg, err := wizard.Run(os.Stdin, os.Stdout)
	if err != nil {
		fail("reconfigure:", err)
	}
	if err := config.Save(config.DefaultPath, cfg); err != nil {
		fail("save config:", err)
	}
	fmt.Println("Перезапускаю сервис…")
	if err := systemd.Restart(); err != nil {
		fail("restart:", err)
	}
	fmt.Println("✓ Конфиг обновлён, сервис перезапущен")
}

// runUninstall tears down everything `install` created. Best-effort —
// keep going past missing files so a half-broken install can still be
// cleaned up. The binary itself is removed last (and only if it lives
// in /usr/local/bin), so we don't accidentally delete a dev build the
// user is running from somewhere else.
func runUninstall() {
	mustRoot()

	// Try to load the config to know where DataDir lives. If the file is
	// missing or unparseable, fall back to the default path — uninstall
	// must still work on a corrupted install.
	dataDir := "/var/lib/goronin"
	if cfg, err := config.Load(config.DefaultPath); err == nil && cfg.DataDir != "" {
		dataDir = cfg.DataDir
	}

	// Best-effort firewall cleanup: try to flush the iptables chain so
	// we don't leave dangling REJECT rules pointing at IPs we've forgotten.
	// We open storage only to call ResetChain, so silently skip if it fails.
	if store, err := storage.Open(dataDir + "/state.db"); err == nil {
		fw := firewall.New(nil, firewall.RealExecutor{}).WithStorage(store)
		_ = fw.ResetChain()
		_ = store.Close()
	}

	fmt.Println("Останавливаю и удаляю systemd unit…")
	if err := systemd.Uninstall(); err != nil {
		fmt.Fprintln(os.Stderr, "  ⚠", err)
	}

	fmt.Println("Удаляю конфиг:", config.DefaultPath)
	if err := os.RemoveAll("/etc/goronin"); err != nil {
		fmt.Fprintln(os.Stderr, "  ⚠ удаление /etc/goronin:", err)
	}

	fmt.Println("Удаляю данные:", dataDir)
	if err := os.RemoveAll(dataDir); err != nil {
		fmt.Fprintln(os.Stderr, "  ⚠ удаление", dataDir+":", err)
	}

	const installedBin = "/usr/local/bin/goronin"
	if binPath, err := os.Executable(); err == nil && binPath == installedBin {
		// We're about to delete the binary we're running — that's fine on
		// Linux (the kernel keeps the inode alive until exit), but flag it
		// so the user understands why `goronin` is gone after this command.
		fmt.Println("Удаляю бинарь:", installedBin)
		if err := os.Remove(installedBin); err != nil && !os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "  ⚠", err)
		}
	} else if _, err := os.Stat(installedBin); err == nil {
		fmt.Println("Удаляю бинарь:", installedBin)
		_ = os.Remove(installedBin)
	}

	fmt.Println()
	fmt.Println("✓ GORONIN полностью удалён")
}

// ---------- daemon ----------

func runDaemon() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("[goronin] starting daemon, version=%s", version)

	cfg, err := config.Load(config.DefaultPath)
	if err != nil {
		log.Fatalf("[goronin] load config: %v (run `goronin install` first)", err)
	}

	// Persistent state across restarts.
	store, err := storage.Open(cfg.DataDir + "/state.db")
	if err != nil {
		log.Fatalf("[goronin] open storage: %v", err)
	}
	defer store.Close()
	_ = store.SetMeta("version", version)
	_ = store.SetMeta("last_start", time.Now().Format(time.RFC3339))

	// Telegram + AI: constructed first so failures surface before traps bind.
	tg := telegram.New(cfg.Telegram)
	provider, err := ai.New(cfg.AI)
	if err != nil {
		log.Fatalf("[goronin] init ai: %v", err)
	}

	// Alerter is the AI/Telegram routing layer. In v0.2+ it receives
	// aggregated batches via FlushBatch; only file-canary events use the
	// instant bypass HandleInstant.
	al := alerter.New(cfg.ServerName, provider, tg)

	// Aggregator: 5-min urgent + 1-hour background, tunable via config.
	agg := aggregator.New(aggregator.Config{
		UrgentWindow:      cfg.Alerting.UrgentWindow,
		BackgroundWindow:  cfg.Alerting.BackgroundWindow,
		InterestThreshold: cfg.Alerting.InterestThreshold,
	}, al.FlushBatch)
	defer agg.Stop()

	// Firewall: persistent blocks, threshold-based RecordHit.
	fw := firewall.New(cfg.WhitelistIPs, firewall.RealExecutor{}).
		WithStorage(store).
		WithPolicy(cfg.AutoBan)
	if err := fw.InitChain(); err != nil {
		log.Printf("[goronin] firewall init warning (active defense disabled): %v", err)
	}
	if err := fw.RestoreFromStorage(); err != nil {
		log.Printf("[goronin] restore blocks: %v", err)
	}
	fw.Start()
	defer fw.Shutdown()

	// onEvent: every trap/watcher event flows through here. Firewall reaction
	// runs first (so the alert reflects what we actually did). Then the
	// event is routed:
	//   - file-canary write/remove → al.HandleInstant (bypass aggregator)
	//   - everything else          → agg.Observe (5-min batching)
	onEvent := func(ev protocol.EventRequest) {
		if ev.Details == nil {
			ev.Details = map[string]string{}
		}
		if ev.SourceIP != "" && ev.SourceIP != "localhost" {
			result := fw.RecordHit(ev.SourceIP, ev.Type)
			ev.Details[protocol.DetailActionTaken] = string(result)
			ev.Details[protocol.DetailBlockReason] = ev.Type
		}

		// File canary on a write/remove is a 100% real attack — don't wait
		// 5 minutes to alert. Read events still go through the aggregator
		// because read can be a false positive (cron, backups).
		if isInstantEvent(ev) {
			al.HandleInstant(ev)
			return
		}
		agg.Observe(ev)
	}

	// Traps.
	tm := traps.NewManager(onEvent)
	if err := tm.StartTraps(cfg.Traps.SSH, cfg.Traps.HTTP, cfg.Traps.FTP, cfg.Traps.DB); err != nil {
		log.Printf("[goronin] start traps: %v", err)
	}

	// File watcher: configured files + auto-discovered secrets + canaries.
	// canaries is hoisted out so we can include it in the startup message.
	var canaries []string
	fileWatcher, err := watcher.New(onEvent)
	if err != nil {
		log.Printf("[goronin] file watcher unavailable: %v", err)
	} else {
		if len(cfg.WatchFiles) > 0 {
			fileWatcher.WatchFiles(cfg.WatchFiles)
		}
		if discovered := watcher.AutoDiscover(); len(discovered) > 0 {
			fileWatcher.WatchFiles(discovered)
		}
		canaries = fileWatcher.CreateCanaries([]string{"/root", "/tmp", "/var/www"})
		if len(canaries) > 0 {
			fileWatcher.WatchFiles(canaries)
		}
		fileWatcher.Start()
		defer fileWatcher.Stop()
	}

	// Startup notification with the actual ports we ended up on.
	running := tm.RunningTraps()
	descs := make([]string, 0, len(running))
	for _, t := range running {
		descs = append(descs, fmt.Sprintf("%s:%d", labelOf(t.Type), t.Port))
	}
	al.SendStartup(version, descs, canaries)

	log.Println("[goronin] daemon ready")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Println("[goronin] shutting down")
	tm.StopAll()
}

// isInstantEvent reports whether an event should bypass the aggregator
// and trigger an immediate Telegram alert. Right now: file_canary write/
// remove operations. A canary read might be a false positive (cron job,
// backup tool) so it still goes through the 5-minute window.
func isInstantEvent(ev protocol.EventRequest) bool {
	if ev.Type != protocol.EventFileCanary {
		return false
	}
	op := ev.Details["operation"]
	return strings.Contains(op, "WRITE") || strings.Contains(op, "REMOVE")
}

func labelOf(typ string) string {
	switch typ {
	case protocol.EventSSHTrap:
		return "ssh"
	case protocol.EventHTTPTrap:
		return "http"
	case protocol.EventFTPTrap:
		return "ftp"
	case protocol.EventDBTrap:
		return "db"
	}
	return typ
}

// ---------- unban / reset ----------

func runUnban() {
	mustRoot()
	if len(os.Args) < 3 {
		fail("usage:", fmt.Errorf("goronin unban <ip>"))
	}
	ip := os.Args[2]

	cfg, err := config.Load(config.DefaultPath)
	if err != nil {
		fail("load config:", err)
	}
	store, err := storage.Open(cfg.DataDir + "/state.db")
	if err != nil {
		fail("open store:", err)
	}
	defer store.Close()

	fw := firewall.New(nil, firewall.RealExecutor{}).WithStorage(store)
	if err := fw.UnblockIP(ip); err != nil {
		fail("unban:", err)
	}
	fmt.Println("✓", ip, "разблокирован")
}

func runReset() {
	mustRoot()
	cfg, err := config.Load(config.DefaultPath)
	if err != nil {
		fail("load config:", err)
	}
	store, err := storage.Open(cfg.DataDir + "/state.db")
	if err != nil {
		fail("open store:", err)
	}
	defer store.Close()

	fw := firewall.New(nil, firewall.RealExecutor{}).WithStorage(store)
	if err := fw.ResetChain(); err != nil {
		fail("reset:", err)
	}
	fmt.Println("✓ iptables-цепочка очищена, активные баны удалены")
}

// ---------- helpers ----------

func mustSystemd(fn func() error) {
	mustRoot()
	if err := fn(); err != nil {
		fail("systemctl:", err)
	}
}

func mustRoot() {
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "Эта команда требует root. Запусти через sudo.")
		os.Exit(1)
	}
}

func fail(prefix string, err error) {
	fmt.Fprintln(os.Stderr, prefix, err)
	os.Exit(1)
}
