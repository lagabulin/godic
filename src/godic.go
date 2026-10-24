// original redis code
// - redis.h
// - redis.c
// - asciilogo.h
package main

import (
	"bufio"
	"flag"
	"fmt"
	"godic-server/ae"
	"godic-server/memtest"
	"godic-server/release"
	"godic-server/version"
	"log/syslog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
	"unsafe"
)

// ========================= Static server configurations
const (
	GodicDefaultHz = 10

	GodicServerPort  = 6379 /* TCP port */
	GodicTcpBacklog  = 511  /* TCP listen backlog */
	GodicMaxidletime = 0    /* default client timeout: infinite */

	GodicDefaultDbnum = 16

	GodicRunIdSize = 40

	GodicDefaultSyslogIdent = "godic"

	GodicDefaultDaemonize      = false
	GodicDefaultUnixSocketPerm = 0
	GodicDefaultTcpKeepalive   = 0
	GodicDefaultLogfile        = ""
	GodicDefaultSyslogEnabled  = false

	GodicDefaultMaxmemory = 0
)

// ========================= Protocol and I/O related defines
const (
	GodicMaxQuerybufLen = (1024 * 1024 * 1024) /* 1GB max query buffer */
)

// ========================= Log levels
type LogLevel uint

const (
	GodicDebug LogLevel = iota
	GodicVerbose
	GodicNotice
	GodicWarning
	GodicLogRaw           LogLevel = (1 << 10)
	GodicDefaultVerbosity LogLevel = GodicNotice
)

// ========================= Types
type timeval struct {
	tvSec  int64
	tvUsec int64
}

type godicServer struct {
	/* General */
	pid        int     /* Main process pid. */
	configfile *string /* Absolute config file path, or NULL */
	hz         int     /* serverCron() calls frequency in hertz */
	// db *godicDb
	// commands *dict;             /* Command table */
	// origCommands *dict;         /* Command table before command renaming. */
	el *ae.EventLoop
	//lruclock uint:GODIC_LRU_BITS; /* Clock for LRU eviction */
	shutdownAsap    int                      /* SHUTDOWN needed ASAP */
	activerehashing int                      /* Incremental rehash in serverCron() */
	requirepass     string                   /* Pass for AUTH command, or NULL */
	pidfile         string                   /* PID file path */
	archBits        int                      /* 32 or 64 depending on sizeof(long) */
	cronloops       int                      /* Number of times the cron function run */
	runid           [GodicRunIdSize + 1]byte /* ID always different at every exec. */
	sentinelMode    bool                     /* True if this instance is a Sentinel. */

	/* Networking */
	port       int /* TCP listening port */
	tcpBacklog int /* TCP listen() backlog */
	// bindaddr [GODIC_BINDADDR_MAX]string; /* Addresses we should bind to */
	bindaddrCount  int    /* Number of addresses in server.bindaddr[] */
	unixsocket     string /* UNIX socket path */
	unixsocketperm uint   /* UNIX socket permission */
	// ipfd [GODIC_BINDADDR_MAX]int; /* TCP socket file descriptors */
	ipfdCount int /* Used slots in ipfd[] */
	sofd      int /* Unix socket file descriptor */
	// cfd [GODIC_BINDADDR_MAX]int;/* Cluster bus listening socket */
	cfdCount int /* Used slots in cfd[] */
	// clients *list;              /* List of active clients */
	// clients_to_close *list;     /* Clients to close asynchronously */
	// slaves, monitors *list;     /* List of slaves and MONITORs */
	// current_client *godicClient; /* Current client, only used on crash report */
	clientsPaused int /* True if clients are currently paused */
	// clientsPauseEndTime mstime_t; /* Time when we undo clients_paused */
	// neterr [ANET_ERR_LEN]byte;   /* Error buffer for anet.c */
	// migrateCachedSockets *dict; /* MIGRATE cached sockets */
	nextClientId uint64 /* Next client unique ID. Incremental. */

	/* Configuration */
	verbosity            LogLevel /* Loglevel in godic.conf */
	maxidletime          int      /* Client timeout in seconds */
	tcpkeepalive         int      /* Set SO_KEEPALIVE if non-zero. */
	activeExpireEnabled  bool     /* Can be disabled for testing purposes. */
	clientMaxQuerybufLen uint64   /* Limit for client query buffer length */
	dbnum                int      /* Total number of configured DBs */
	daemonize            bool     /* True if running as a daemon */

	/* Logging */
	logfile        string          /* Path of log file */
	syslogEnabled  bool            /* is syslog enabled? */
	syslogIdent    string          /* Syslog ident */
	syslogFacility syslog.Priority /* Syslog facility */
	syslogWriter   *syslog.Writer  /* Valid only if syslogEnabled*/

	/* Replication (slave) */
	masterhost *string /* Hostname of master */

	/* Limits */
	maxmemory uint64 /* Max number of memory bytes to use */
}

// ========================= Global variables
var server godicServer

// ========================= Functions
func printVersion() {
	var machineBits int = 64
	if unsafe.Sizeof(int(0)) == 4 {
		machineBits = 32
	}

	dirty, _ := strconv.Atoi(release.GodicGitDirty)

	// Godic does not use cumstomized allocators like zmalloc.
	// TODO(BUILD): add build_id and crc64
	fmt.Printf("Godic server v=%s sha=%s:%d bits=%d\n",
		version.GodicVersion,
		release.GodicGitSHA1,
		dirty,
		machineBits,
	)

	os.Exit(1)
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: ./godic-server [/path/to/godic.conf] [options]\n")
	fmt.Fprintf(os.Stderr, "       ./godic-server - (read config from stdin)\n")
	fmt.Fprintf(os.Stderr, "       ./godic-server -v or --version\n")
	fmt.Fprintf(os.Stderr, "       ./godic-server -h or --help\n")
	fmt.Fprintf(os.Stderr, "Example:\n")
	fmt.Fprintf(os.Stderr, "       ./godic-server (run the server with default conf)\n")
	fmt.Fprintf(os.Stderr, "       ./godic-server /etc/godic/6379.conf\n")
	fmt.Fprintf(os.Stderr, "       ./godic-server --port 7777\n")
	fmt.Fprintf(os.Stderr, "       ./godic-server --port 7777 --slaveof 127.0.0.1 8888\n")
	fmt.Fprintf(os.Stderr, "       ./godic-server /etc/mygodic.conf --loglevel verbose\n")
	// fmt.Fprintf(os.Stderr, "Sentinel mode:\n")
	// fmt.Fprintf(os.Stderr, "       ./godic-server /etc/sentinel.conf --sentinel \n")
	os.Exit(1)
}

// TODO(LOG): add documentation for godicLog
func godicLog(level LogLevel, format string, a ...interface{}) {
	if (level & 0xff) < server.verbosity {
		return
	}
	msg := fmt.Sprintf(format, a...)
	godicLogRaw(level, msg)
}

func godicLogRaw(level LogLevel, msg string) {
	var c string = ".-*#"
	var rawmode bool = (int(level&GodicLogRaw) != 0)
	var logToStdout bool = (server.logfile == "")

	level &= 0xff /* clear flags */
	if level < server.verbosity {
		return
	}

	var fp *os.File
	if logToStdout {
		fp = os.Stdout
	} else {
		var err error
		fp, err = os.OpenFile(server.logfile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
		defer fp.Close()
	}

	writer := bufio.NewWriter(fp)

	if rawmode {
		writer.WriteString(msg)
	} else {
		var roleChar rune
		var pid int = os.Getpid()

		prefix := time.Now().Format("2006-01-02T15:04:05.000 -0700")

		if server.sentinelMode {
			roleChar = 'X'
		} else if pid != server.pid {
			roleChar = 'C'
		} else {
			if nil == server.masterhost {
				roleChar = 'S'
			} else {
				roleChar = 'M'
			}
		}

		fullMsg := fmt.Sprintf("%d:%c %s %c %s\n", pid, roleChar, prefix, c[level], msg)
		writer.WriteString(fullMsg)
	}
	writer.Flush()

	if server.syslogEnabled {
		switch level {
		case GodicDebug:
			server.syslogWriter.Debug(msg)
		case GodicVerbose:
			server.syslogWriter.Info(msg)
		case GodicNotice:
			server.syslogWriter.Notice(msg)
		case GodicWarning:
			server.syslogWriter.Warning(msg)
		default:
			// early crash for noticing places to modify according to additional levels
			server.syslogWriter.Info("No branch for this level.\n")
			server.syslogWriter.Info(msg)
		}
	}
}

func daemonize() {
	if os.Getenv("GODIC_DAEMONIZED") != "" {
		// child process code goes here
		syscall.Setsid()
		devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
		if err != nil {
			return
		}
		/* Every ouput goes to /dev/null. If Godic is daemonized but
		 * the 'logfile' is set to 'stdout' in the configuration file
		 * it will not log at all.
		 * The process has no associated terminal, so interacting with
		 * stdio could cause SIGPIPE or blocking. */
		syscall.Dup2(int(devNull.Fd()), int(os.Stdin.Fd()))
		syscall.Dup2(int(devNull.Fd()), int(os.Stdout.Fd()))
		syscall.Dup2(int(devNull.Fd()), int(os.Stderr.Fd()))

		// Close devNull only if it was not open with stdio fd.
		if devNull.Fd() < os.Stderr.Fd() {
			defer devNull.Close()
		}
		return
	}

	cmd := exec.Command(os.Args[0], os.Args[1:]...)
	cmd.Env = append(os.Environ(), "GODIC_DAEMONIZED=1")
	cmd.Start()
	os.Exit(0)
}

func createPidFile() {
	// TODO(DAEMON): write PID to pidfile
}

func godicSetProcTitle(title string) {
	// TODO(PROCESS): set process title via prctl or similar
}

func godicAsciiArt() {
    ascii := `
  *#####*     *#####*         #######*   ###   *#####*
 ##*-----*   ##*---*##   +#*  ###---+##  ###  ##*----#+
###         ###      ### *#*  ###   ###  ### ###
###   ##### ###      ###      ###   ###  ### ###
###*    ### ###*    *### *#*  ###   -##+ ### ###*
 *######-+   -*#####*-   +-+  #######*-  ###  +#####*+
   -----+      -----          -------    ---    -----
`

	var mode string
	// TODO(CLUSTER): add clusterEnabled check
	if server.sentinelMode {
		mode = "sentinel"
	} else {
		mode = "standalone"
	}

	dirty, _ := strconv.Atoi(release.GodicGitDirty)
	var bits int
	if unsafe.Sizeof(int(0)) == 8 {
		bits = 64
	} else {
		bits = 32
	}

	if server.syslogEnabled {
		godicLog(GodicNotice,
			"Godic %s (%s/%d) %d bit, %s mode, port %d, pid %d ready to start.",
			version.GodicVersion,
			release.GodicGitSHA1,
			dirty,
			bits,
			mode, server.port,
			os.Getpid(),
		)
	} else {
		godicLogRaw(GodicNotice|GodicLogRaw, ascii)
		godicLog(GodicNotice,
			"Godic %s (%s/%d) %d bit, %s mode, port %d, pid %d ready to start.",
			version.GodicVersion,
			release.GodicGitSHA1,
			dirty,
			bits,
			mode, server.port,
			os.Getpid(),
		)
	}
}

func checkTcpBacklogSettings() {
	// TODO(NETWORK): check /proc/sys/net/core/somaxconn vs tcpBacklog
}

func initDaemonConfig() {
	server.daemonize = GodicDefaultDaemonize
}

func initServerConfig() {
	// getRandomHexChars(server.runid, GODIC_RUN_ID_SIZE);
	server.hz = GodicDefaultHz
	if unsafe.Sizeof(int(0)) == 8 {
		server.archBits = 64
	} else {
		server.archBits = 32
	}
	server.port = GodicServerPort
	server.tcpBacklog = GodicTcpBacklog
	server.bindaddrCount = 0
	server.unixsocketperm = GodicDefaultUnixSocketPerm
	server.ipfdCount = 0
	server.sofd = -1
	server.dbnum = GodicDefaultDbnum
	server.verbosity = GodicDefaultVerbosity
	server.maxidletime = GodicMaxidletime
	server.tcpkeepalive = GodicDefaultTcpKeepalive
	server.activeExpireEnabled = true
	server.clientMaxQuerybufLen = GodicMaxQuerybufLen

	server.logfile = GodicDefaultLogfile
	server.syslogEnabled = GodicDefaultSyslogEnabled
	server.syslogIdent = GodicDefaultSyslogIdent
	server.syslogFacility = syslog.LOG_LOCAL0
	if server.syslogEnabled {
		writer, err := syslog.New(syslog.LOG_INFO|server.syslogFacility, server.syslogIdent)
		if err != nil {
			server.syslogEnabled = false
		} else {
			server.syslogWriter = writer
		}
	}

	server.maxmemory = GodicDefaultMaxmemory
}

func initServer() {
}

// ========================= Main
func main() {
	/* initDaemonConfig() should be called in advance of the other
	 * initailization steps. Golang does not support direct invocations
	 * of fork(), so we daemonize the server first to be free from
	 * idempotence guarantee. */
	initDaemonConfig()
	if server.daemonize {
		daemonize()
	}

	argc := len(os.Args)

	// spt_init(argc, argv);	  // TODO(PROCESS)
	// setlocale(LC_COLLATE, ""); // TODO(INIT)
	// zmalloc_set_oom_handler(godicOutOfMemoryHandler) // TODO(MEMORY)
	// tv := timeval{time.Now().Unix(), time.Now().UnixMicro()}
	// dictSetHashFuncionSeed(tv.tvSec ^ tv.tvUsec ^ int64(os.GetPid())) // TODO(DICT)
	// server.sentinelMode = checkForSentinelMode(argc, argv) // TODO(SENTINEL)
	server.sentinelMode = false
	initServerConfig()

	/* We need to init sentinel right now as parsing the configuration file
	 * in sentinel mode will hvae the effect of populating the sentinel
	 * data structures with master nodes to monitor */
	if server.sentinelMode {
		/* // TODO(SENTINEL): initialize sentinel config and state
		initSentinelConfig()
		initSentinel()
		*/
	}

	if argc >= 2 {
		var j int = 1
		var options string = ""
		var configfile *string = nil

		// short options go here
		vPtr := flag.Bool("v", false, "version")
		hPtr := flag.Bool("h", false, "help")

		// long options go here
		versionPtr := flag.Bool("version", false, "version")
		helpPtr := flag.Bool("help", false, "help")
		testmemoryPtr := flag.Int("test-memory", 0, "memory size to test")

		flag.Parse()

		if true == *vPtr || true == *versionPtr {
			printVersion()
		}

		if true == *hPtr || true == *helpPtr {
			printUsage()
		}

		if 0 == *testmemoryPtr {
			if argc == 3 {
				memsize, err := strconv.Atoi(os.Args[2])
				if err != nil {
					fmt.Fprintf(os.Stderr, "Failed to parse the amount of memory to test. err: %s\n", err)
					os.Exit(1)
				}
				memtest.Memtest(uint64(memsize), 50) // TODO(MEMTEST)
				os.Exit(0)
			} else {
				fmt.Fprintf(os.Stderr, "Please specify the amount of memory to test in megabytes.\n")
				fmt.Fprintf(os.Stderr, "Example: ./godic-server --test-memory 4096\n")
				os.Exit(1)
			}
		}

		/* First argument is the config file name? */
		if os.Args[j][0] != '-' || os.Args[j][1] != '-' {
			configfile = &os.Args[j]
			j++
		}

		/* All the other options are parsed and conceptually appended to the
		 * configuration file. For instatnce, --port 6380 will generate the
		 * string "port 6380\n" to be parsed after the actual file name
		 * is parsed, if any. */
		for j != argc {
			if os.Args[j][0] == '-' && os.Args[j][1] == '-' {
				/* Option name */
				if 0 == len(options) {
					options = options + "\n"
				}
				options = options + os.Args[j] + " "
			} else {
				/* Option argument */
				// TODO(CONFIG): implement sdscatrepr
				// options = sdscatrepr(options, os.Args[j], len(os.Args[j]))
				options = options + " "
			}
			j++
		}

		if server.sentinelMode && nil != configfile && (*configfile)[0] == '-' {
			godicLog(GodicWarning,
				"Sentinel config from STDIN not allowed.")
			godicLog(GodicWarning,
				"Sentinel needs config file on disk to save state.  Exiting...")
			os.Exit(1)
		}

		if nil != configfile {
			absPath, err := filepath.Abs(*configfile)
			if err != nil {
				server.configfile = nil
			} else {
				server.configfile = &absPath
			}
		}
		// resetServerSaveParams() // TODO(RDB): reset save params before config load
		// loadServerConfig(configfile, options) // TODO(CONFIG)

	} else {
		var confName string
		if server.sentinelMode {
			confName = "sentinel"
		} else {
			confName = "godic"
		}
		godicLog(GodicWarning, "Warning: no config file specified, using the default config. In order to specify a config file use %s /path/to/%s.conf", os.Args[0], confName)
	}

	initServer() // TODO(INIT)
	if server.daemonize {
		createPidFile() // TODO(DAEMON)
	}
	godicSetProcTitle(os.Args[0]) // TODO(PROCESS)
	godicAsciiArt()
	checkTcpBacklogSettings()     // TODO(NETWORK)

	if !server.sentinelMode {
		godicLog(GodicWarning, "Server started, Godic version %s", version.GodicVersion)
		/*
			#ifdef __linux__
				linuxMemoryWarnings() // TODO(MEMORY)
			#endif
				loadDataFromDisk() // TODO(PERSISTENCE): load RDB or AOF on startup
		*/
		/*
			if server.cluster_enabled {
				// TODO(CLUSTER): verify cluster state after data load
			}
		*/

		if server.ipfdCount > 0 {
			godicLog(GodicNotice, "The server is now ready to accept connections on port %d", server.port)
		}

		if server.sofd > 0 {
			godicLog(GodicNotice, "The server is now ready to accept connections at %s", server.unixsocket)
		}
	} else {
		// TODO(SENTINEL): sentinelIsRunning()
	}

	/* Warning the user about suspicious maxmemory setting. */
	if 0 < server.maxmemory && server.maxmemory < (1024*1024) {
		godicLog(GodicWarning, "WARNING: You specified a maxmemory value that is less than 1MB (current value is %llu bytes). Are you sure this is what you really want?", server.maxmemory)
	}

	server.el.Main()            // TODO(EVENT_LOOP)
	server.el.DeleteEventLoop() // TODO(EVENT_LOOP)
}
