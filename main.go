package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/op/go-logging"
)

var Log = logging.MustGetLogger("tvhtc")
var logFormat = logging.MustStringFormatter("[%{time:15:04:05.000}] %{level:.4s} (%{shortfunc}) - %{message}")
var syslogFormat = logging.MustStringFormatter("%{level:.4s} (%{shortfunc}) - %{message}")

func main() {
	var configPath string
	flag.StringVar(&configPath, "c", "/etc/tvhtc.conf", "Path to configuration file")
	var debug bool
	flag.BoolVar(&debug, "d", false, "Enable debugging output to stdout")
	flag.Parse()

	if !debug {
		sb, err := logging.NewSyslogBackend("tvhtc")
		if err != nil {
			fmt.Printf("Fatal error setting up syslog logging: %v", err)
			os.Exit(1)
		}
		logging.SetBackend(sb)
		logging.SetLevel(logging.INFO, "tvhtc")
		logging.SetFormatter(syslogFormat)
	} else {
		logging.SetBackend(logging.NewLogBackend(os.Stderr, "", 0))
		logging.SetLevel(logging.DEBUG, "tvhtc")
		logging.SetFormatter(logFormat)
	}

	Log.Warning("TVHTC starting up. Using config file: %v", configPath)

	rand.Seed(time.Now().Unix())
	config := NewConfig()
	if err := config.Load(configPath); err != nil {
		fmt.Printf(err.Error())
		os.Exit(1)
	}

	g := gin.Default()
	if !debug {
		gin.SetMode("release")
	}
	g.POST("/job", func(c *gin.Context) {
		job := TVHJob{}
		c.Bind(&job)
		Log.Warning("Got new transcode job: %+v", job)
		Transcode(&job)
		c.JSON(200, gin.H{"status": "ok"})
		return
	})

	g.GET("/job", func(c *gin.Context) {
		//get list of all jobs etc
	})

	g.GET("/memstats", func(c *gin.Context) {
		// memory stats
		ms := runtime.MemStats{}
		runtime.ReadMemStats(&ms)
		c.JSON(200, gin.H{"total_alloc": ms.TotalAlloc, "in_use": ms.Alloc})
		return
	})

	StartQueueManager(&config)
	g.Run(":8998")
}
