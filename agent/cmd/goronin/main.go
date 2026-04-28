// goronin — standalone honeypot guard.
//
// Subcommands:
//   install      — interactive wizard, writes config.yml + systemd unit, starts service
//   reconfigure  — re-runs the wizard, restarts the service
//   uninstall    — stop service, remove unit + binary + config + data
//   daemon       — the actual long-running process (called by systemd, not the user)
//   start/stop/restart/status/logs — systemd wrappers
//   health       — quick green/red check across all subsystems
//   unban <ip>   — remove an IP from the GORONIN-BLOCK chain
//   reset        — flush iptables chain and clear persisted blocks
//   version      — print build info
//
// Bare invocation prints usage. The `daemon` mode is invoked by the systemd
// unit and is what actually listens for traps and sends alerts.
package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
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
	case "health":
		runHealth()
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
  goronin health               проверка состояния всех подсистем
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

	// Wire firewall into alerter so batch alerts can show "🛡 заблокирован" markers.
	al.WithFirewall(fw)

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
	// canaries is hoisted so the startup message can show it. We pass
	// Created+Existing (every canary actually under inotify), not only newly
	// created — so a restart on a box already set up correctly reports the
	// canaries as active rather than "not created".
	var canaries []string
	var canaryFailed []string
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
		canaryRes := fileWatcher.CreateCanaries(watcher.CanaryDirs)
		canaries = canaryRes.All()
		canaryFailed = canaryRes.Failed
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
	al.SendStartup(version, descs, canaries, canaryFailed)

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

// ---------- health ----------

// runHealth prints a green/red checklist across the things that can break:
// systemd state, config, traps (last reported ports + currently listening),
// canaries, iptables chain, telegram reachability, AI provider config,
// recent error count. Best-effort — every check catches its own errors so
// the report always finishes and shows what worked alongside what didn't.
func runHealth() {
	mustRoot()

	const (
		cReset = "\x1b[0m"
		cOK    = "\x1b[32m"
		cWarn  = "\x1b[33m"
		cErr   = "\x1b[31m"
		cDim   = "\x1b[2m"
		cBold  = "\x1b[1m"
	)
	useColor := isTerminal(os.Stdout)
	color := func(c, s string) string {
		if !useColor {
			return s
		}
		return c + s + cReset
	}

	fmt.Printf("\n%s GORONIN health check%s\n\n", color(cBold, ""), color("", ""))

	ok := func(label, value string) {
		fmt.Printf("  %s %s         %s\n", color(cOK, "✓"), padRight(label, 12), value)
	}
	warn := func(label, value string) {
		fmt.Printf("  %s %s         %s\n", color(cWarn, "⚠"), padRight(label, 12), value)
	}
	bad := func(label, value string) {
		fmt.Printf("  %s %s         %s\n", color(cErr, "✗"), padRight(label, 12), value)
	}

	overallOK := true

	// --- 1. Service state + uptime ---
	if systemd.IsActive() {
		uptime := serviceUptime()
		if uptime != "" {
			ok("сервис", fmt.Sprintf("active (running) — uptime %s", uptime))
		} else {
			ok("сервис", "active (running)")
		}
	} else {
		bad("сервис", "не запущен — попробуй: sudo goronin start")
		overallOK = false
	}

	// --- 2. Config ---
	cfg, cfgErr := config.Load(config.DefaultPath)
	if cfgErr != nil {
		bad("конфиг", fmt.Sprintf("не загружается: %v", cfgErr))
		fmt.Println()
		fmt.Println(color(cDim, "  Без конфига дальнейшие проверки невозможны. Запусти: sudo goronin install"))
		os.Exit(1)
	}
	ok("конфиг", config.DefaultPath)

	// --- 3. Version ---
	ok("версия", version)

	// --- 4. Traps: last reported ports from journal + which are still listening ---
	ports := readLastTrapPorts()
	if len(ports) == 0 {
		warn("ловушки", "не найдены в логах — сервис только что стартовал?")
	} else {
		fmt.Printf("  %s %s\n", color(cOK, "✓"), padRight("ловушки", 12))
		for _, p := range ports {
			alive := isPortListening(p.port)
			marker := color(cOK, "✓")
			tail := ""
			if !alive {
				marker = color(cErr, "✗")
				tail = "  (порт не слушает!)"
				overallOK = false
			}
			fmt.Printf("                  %s  %-5s %d%s\n", marker, p.kind, p.port, tail)
		}
	}

	// --- 5. Canaries ---
	// Match what the daemon actually watches: configured files + auto-discovered
	// secrets + the canary grid (CanaryDirs × CanaryNames). Files that don't
	// exist on disk are counted as missing so a missing /root/.aws_credentials
	// shows up here, not silently.
	expected := collectExpectedWatchedFiles(cfg.WatchFiles)
	if len(expected) == 0 {
		warn("канарейки", "ни одного отслеживаемого файла")
	} else {
		alive, missing := 0, 0
		for _, f := range expected {
			if _, err := os.Stat(f); err == nil {
				alive++
			} else {
				missing++
			}
		}
		msg := fmt.Sprintf("%d файлов отслеживается", alive)
		if missing > 0 {
			warn("канарейки", fmt.Sprintf("%s, %d отсутствует", msg, missing))
		} else {
			ok("канарейки", msg)
		}
	}

	// --- 6. Firewall: GORONIN-BLOCK chain + active blocks count ---
	chainExists, blockCount := firewallStatus(cfg.DataDir)
	switch {
	case !chainExists:
		warn("iptables", "цепочка GORONIN-BLOCK не найдена (auto_ban=off?)")
	case blockCount == 0:
		ok("iptables", "цепочка активна, активных банов нет")
	default:
		ok("iptables", fmt.Sprintf("цепочка активна, %d IP заблокировано", blockCount))
	}

	// --- 7. Telegram: getMe ---
	tg := telegram.New(cfg.Telegram)
	tgCtx, tgCancel := context.WithTimeout(context.Background(), 5*time.Second)
	botName, tgErr := tg.Verify(tgCtx)
	tgCancel()
	if tgErr != nil {
		bad("telegram", fmt.Sprintf("getMe failed: %v", tgErr))
		overallOK = false
	} else {
		ok("telegram", "@"+botName)
	}

	// --- 8. AI provider ---
	switch strings.ToLower(cfg.AI.Provider) {
	case "", "none", "off":
		warn("AI", "отключён (анализ событий без рассуждений)")
	default:
		model := cfg.AI.Model
		if model == "" {
			model = "(дефолтная модель)"
		}
		if cfg.AI.APIKey == "" {
			bad("AI", fmt.Sprintf("%s — API-ключ пуст", cfg.AI.Provider))
			overallOK = false
		} else {
			ok("AI", fmt.Sprintf("%s · %s", cfg.AI.Provider, model))
		}
	}

	// --- 9. Errors in last hour ---
	errCount := countRecentErrors(time.Hour)
	switch {
	case errCount < 0:
		warn("ошибки", "не удалось прочитать journal")
	case errCount == 0:
		ok("ошибки", "за последний час: 0")
	case errCount < 5:
		warn("ошибки", fmt.Sprintf("за последний час: %d (см. goronin logs)", errCount))
	default:
		bad("ошибки", fmt.Sprintf("за последний час: %d (см. goronin logs)", errCount))
		overallOK = false
	}

	fmt.Println()
	if overallOK {
		fmt.Println(" ", color(cOK, "всё ок"))
	} else {
		fmt.Println(" ", color(cErr, "есть проблемы — см. строки с ✗ выше"))
	}
	fmt.Println()
	if !overallOK {
		os.Exit(1)
	}
}

// serviceUptime returns a human-readable uptime ("1h 24m", "3d 2h", "45s")
// for the goronin systemd unit. Pulls ActiveEnterTimestamp via `systemctl show`.
// Returns "" on any parsing failure — the caller falls back to a plain
// "active (running)" line so the health check never breaks because of this.
func serviceUptime() string {
	out, err := exec.Command("systemctl", "show", systemd.ServiceName, "--property=ActiveEnterTimestamp").Output()
	if err != nil {
		return ""
	}
	// Output: "ActiveEnterTimestamp=Tue 2026-04-28 02:50:53 UTC"
	line := strings.TrimSpace(string(out))
	idx := strings.Index(line, "=")
	if idx < 0 || idx == len(line)-1 {
		return ""
	}
	stamp := strings.TrimSpace(line[idx+1:])
	if stamp == "" {
		return ""
	}
	// systemd's format: "Tue 2026-04-28 02:50:53 UTC"
	t, err := time.Parse("Mon 2006-01-02 15:04:05 MST", stamp)
	if err != nil {
		return ""
	}
	d := time.Since(t)
	if d < 0 {
		return ""
	}

	days := int(d / (24 * time.Hour))
	hours := int((d % (24 * time.Hour)) / time.Hour)
	mins := int((d % time.Hour) / time.Minute)
	secs := int((d % time.Minute) / time.Second)

	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, mins)
	case mins > 0:
		return fmt.Sprintf("%dm %ds", mins, secs)
	default:
		return fmt.Sprintf("%ds", secs)
	}
}

// collectExpectedWatchedFiles returns the union of files the daemon watches:
// configured watch_files, AutoDiscover hits, and the canary grid (every
// CanaryDir × CanaryName combination). Used by `goronin health` to compare
// against what's actually on disk. Deduplicated, order-stable.
func collectExpectedWatchedFiles(configured []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	for _, p := range configured {
		add(p)
	}
	for _, p := range watcher.AutoDiscover() {
		add(p)
	}
	for _, dir := range watcher.CanaryDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}
		for _, name := range watcher.CanaryNames {
			add(dir + "/" + name)
		}
	}
	return out
}

// trapPort is one row of "last reported trap listeners" parsed from journal.
type trapPort struct {
	kind string // ssh / http / ftp / db
	port int
}

// readLastTrapPorts pulls the most recent "trap listening on port N" lines
// from journalctl for the goronin unit. We only keep one entry per kind
// (the latest), since restarts produce stale lines that would confuse the
// reader. Returns empty slice on any failure — health() degrades gracefully.
func readLastTrapPorts() []trapPort {
	out, err := exec.Command("journalctl", "-u", systemd.ServiceName, "-o", "cat", "--no-pager", "-n", "200").CombinedOutput()
	if err != nil {
		return nil
	}
	re := regexp.MustCompile(`\[traps\]\s+(\w+)_trap\s+trap listening on port\s+(\d+)`)
	latest := map[string]int{}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		m := re.FindStringSubmatch(scanner.Text())
		if m == nil {
			continue
		}
		var p int
		fmt.Sscanf(m[2], "%d", &p)
		latest[m[1]] = p
	}
	order := []string{"ssh", "http", "ftp", "db"}
	out2 := make([]trapPort, 0, 4)
	for _, k := range order {
		if p, ok := latest[k]; ok {
			out2 = append(out2, trapPort{kind: k, port: p})
		}
	}
	return out2
}

// isPortListening checks whether ANY process on the host has the port in
// LISTEN state on tcp4 or tcp6. Uses `ss -ltn` (busybox-friendly) with
// a fallback to `netstat -ltn` for old systems. Returns false on error —
// the health check just won't tick the row green.
func isPortListening(port int) bool {
	tools := [][]string{
		{"ss", "-ltn"},
		{"netstat", "-ltn"},
	}
	needle := fmt.Sprintf(":%d ", port)
	for _, t := range tools {
		out, err := exec.Command(t[0], t[1:]...).CombinedOutput()
		if err != nil {
			continue
		}
		if strings.Contains(string(out), needle) {
			return true
		}
		// Some kernels print "LISTEN" with no trailing space; check end-of-line too.
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, fmt.Sprintf(":%d", port)) && strings.Contains(strings.ToUpper(line), "LISTEN") {
				return true
			}
		}
	}
	return false
}

// firewallStatus reports whether the GORONIN-BLOCK chain exists in iptables
// and how many blocks the storage layer has on record. Both checks are
// independent — chain may exist with zero entries (clean state).
func firewallStatus(dataDir string) (chainExists bool, blockCount int) {
	if err := exec.Command("iptables", "-L", firewall.ChainName, "-n").Run(); err == nil {
		chainExists = true
	}
	if dataDir == "" {
		return chainExists, 0
	}
	store, err := storage.Open(dataDir + "/state.db")
	if err != nil {
		return chainExists, 0
	}
	defer store.Close()
	blocks, err := store.ListBlocks()
	if err != nil {
		return chainExists, 0
	}
	now := time.Now()
	for _, b := range blocks {
		if b.ExpiresAt.After(now) {
			blockCount++
		}
	}
	return chainExists, blockCount
}

// countRecentErrors greps journal for "error"/"failed"/"panic" since `since`
// ago. Returns -1 if journalctl isn't available or fails.
func countRecentErrors(since time.Duration) int {
	mins := int(since.Minutes())
	if mins < 1 {
		mins = 1
	}
	out, err := exec.Command(
		"journalctl", "-u", systemd.ServiceName, "-o", "cat", "--no-pager",
		"--since", fmt.Sprintf("%d min ago", mins),
	).CombinedOutput()
	if err != nil {
		return -1
	}
	re := regexp.MustCompile(`(?i)\b(error|failed|panic)\b`)
	count := 0
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		// Skip our own [traps] startup lines, which contain "trap" but no error.
		if re.MatchString(line) {
			count++
		}
	}
	return count
}

// isTerminal returns true if the file is a TTY. Used to decide on color.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
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
