package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/log"
)

const namespace = "mesos"
const concurrentFetch = 100

// Commandline flags.
var (
	addr         = flag.String("web.listen-address", ":9105", "Address to listen on for web interface and telemetry")
	autoDiscover = flag.Bool("exporter.discovery", false, "Discover all Mesos slaves")
	localURL     = flag.String("exporter.local-url", "http://127.0.0.1:5051", "URL to the local Mesos slave")
	masterURL    = flag.String("exporter.discovery.master-url", "http://mesos-master.example.com:5050", "Mesos master URL")
	metricsPath  = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics")
)

func newTaskMetric(name string, descr string) *prometheus.Desc {
	fqn := prometheus.BuildFQName(namespace, "task", name)
	return prometheus.NewDesc(fqn, descr, taskVariableLabels, nil)
}

var (
	taskVariableLabels = []string{"task", "slave", "framework_id"}

	taskCpuLimitDesc       = newTaskMetric("cpus_limit", "Fractional CPU limit")
	taskCpuNrPeriodsDesc   = newTaskMetric("cpus_nr_periods", "Cumulative CPU periods.")
	taskCpuNrThrottledDesc = newTaskMetric("cpus_nr_throttled", "Cumulative throttled CPU periods.")
	taskCpuSysDesc         = newTaskMetric("cpus_system_time_secs", "Cumulative system CPU time in seconds.")
	taskCpuThrottledDesc   = newTaskMetric("cpus_throttled_time_secs", "Cumulative throttled CPU time in seconds.")
	taskCpuUsrDesc         = newTaskMetric("cpu_user_time_secs", "Cumulative user CPU time in seconds.")
	taskMemAnonDesc        = newTaskMetric("memory_anon_bytes", "Task memory anonymous usage in bytes.")
	taskMemFileDesc        = newTaskMetric("memory_file_bytes", "Task memory file usage in bytes.")
	taskMemMappedDesc      = newTaskMetric("memory_mapped_bytes", "Task memory mapped usage in bytes.")
	taskMemLimitDesc       = newTaskMetric("memory_limit_bytes", "Task memory limit in bytes.")
	taskMemRssDesc         = newTaskMetric("memory_rss_bytes", "Task memory RSS usage in bytes.")
	taskNetRxBytes         = newTaskMetric("net_rx_bytes", "Network received bytes.")
	taskNetRxDropped       = newTaskMetric("net_rx_dropped", "Network received packets dropped.")
	taskNetRxErrors        = newTaskMetric("net_rx_errors", "Network received packet errors.")
	taskNetRxPackets       = newTaskMetric("net_rx_packets", "Network received packets.")
	taskNetTxBytes         = newTaskMetric("net_tx_bytes", "Network sent bytes.")
	taskNetTxDropped       = newTaskMetric("net_tx_dropped", "Network sent packets dropped.")
	taskNetTxErrors        = newTaskMetric("net_tx_errors", "Network sent packets ererors.")
	taskNetTxPackets       = newTaskMetric("net_tx_packets", "Network sent packets.")
)

var httpClient = http.Client{
	Transport: &http.Transport{
		MaxIdleConnsPerHost:   2,
		ResponseHeaderTimeout: 5 * time.Second,
		Dial: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 5 * time.Second,
		}).Dial,
	},
}

type exporterOpts struct {
	autoDiscover bool
	localURL     string
	masterURL    string
}

type exporter struct {
	sync.Mutex
	errors *prometheus.CounterVec
	opts   *exporterOpts
	slaves struct {
		sync.Mutex
		urls []string
	}
}

func newMesosExporter(opts *exporterOpts) *exporter {
	e := &exporter{
		errors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: prometheus.BuildFQName(namespace, "", "exporter"),
				Name:      "slave_scrape_errors_total",
				Help:      "Current total scrape errors",
			},
			[]string{"slave"},
		),
		opts: opts,
	}
	e.slaves.urls = []string{e.opts.localURL}

	if e.opts.autoDiscover {
		log.Info("auto discovery enabled from command line flag.")

		// Update nr. of mesos slaves every 10 minutes
		e.updateSlaves()
		go runEvery(e.updateSlaves, 10*time.Minute)
	}

	return e
}

func (e *exporter) Describe(ch chan<- *prometheus.Desc) {
	e.errors.MetricVec.Describe(ch)
}

func (e *exporter) Collect(ch chan<- prometheus.Metric) {
	e.Lock()
	defer e.Unlock()

	metricsChan := make(chan prometheus.Metric)
	go e.scrapeSlaves(metricsChan)

	for metric := range metricsChan {
		ch <- metric
	}

	e.errors.MetricVec.Collect(ch)
}

func (e *exporter) fetch(urlChan <-chan string, metricsChan chan<- prometheus.Metric, wg *sync.WaitGroup) {
	defer wg.Done()

	for u := range urlChan {
		u, err := url.Parse(u)
		if err != nil {
			log.Warn("could not parse slave URL: ", err)
			continue
		}

		host, _, err := net.SplitHostPort(u.Host)
		if err != nil {
			log.Warn("could not parse network address: ", err)
			continue
		}

		monitorURL := fmt.Sprintf("%s/monitor/statistics.json", u)
		resp, err := httpClient.Get(monitorURL)
		if err != nil {
			log.Warn(err)
			e.errors.WithLabelValues(host).Inc()
			continue
		}
		defer resp.Body.Close()

		var stats []Monitor
		if err = json.NewDecoder(resp.Body).Decode(&stats); err != nil {
			log.Warn("failed to deserialize response: ", err)
			e.errors.WithLabelValues(host).Inc()
			continue
		}

		report := func(mon *Monitor, desc *prometheus.Desc, value float64) {
			metricsChan <- prometheus.MustNewConstMetric(
				desc,
				prometheus.GaugeValue,
				value,
				mon.Source, host, mon.FrameworkId,
			)
		}

		for _, mon := range stats {
			stats := mon.Statistics
			report(&mon, taskCpuLimitDesc, float64(stats.CpusLimit))
			report(&mon, taskCpuNrPeriodsDesc, float64(stats.CpusNrPeriods))
			report(&mon, taskCpuNrThrottledDesc, float64(stats.CpusNrThrottled))
			report(&mon, taskCpuSysDesc, float64(stats.CpusSystemTimeSecs))
			report(&mon, taskCpuThrottledDesc, float64(stats.CpusThrottledTimeSecs))
			report(&mon, taskCpuUsrDesc, float64(stats.CpusUserTimeSecs))
			report(&mon, taskMemAnonDesc, float64(stats.MemAnonBytes))
			report(&mon, taskMemFileDesc, float64(stats.MemFileBytes))
			report(&mon, taskMemLimitDesc, float64(stats.MemLimitBytes))
			report(&mon, taskMemMappedDesc, float64(stats.MemMappedBytes))
			report(&mon, taskMemRssDesc, float64(stats.MemRssBytes))
			report(&mon, taskNetRxBytes, float64(stats.NetRxBytes))
			report(&mon, taskNetRxDropped, float64(stats.NetRxDropped))
			report(&mon, taskNetRxErrors, float64(stats.NetRxErrors))
			report(&mon, taskNetRxPackets, float64(stats.NetRxPackets))
			report(&mon, taskNetTxBytes, float64(stats.NetTxBytes))
			report(&mon, taskNetTxDropped, float64(stats.NetTxDropped))
			report(&mon, taskNetTxErrors, float64(stats.NetTxErrors))
			report(&mon, taskNetTxPackets, float64(stats.NetTxPackets))
		}
	}
}

func (e *exporter) scrapeSlaves(ch chan<- prometheus.Metric) {
	defer close(ch)

	e.slaves.Lock()
	urls := make([]string, len(e.slaves.urls))
	copy(urls, e.slaves.urls)
	e.slaves.Unlock()

	urlCount := len(urls)
	log.Debugf("active slaves: %d", urlCount)

	urlChan := make(chan string)

	poolSize := concurrentFetch
	if urlCount < concurrentFetch {
		poolSize = urlCount
	}

	log.Debugf("creating fetch pool of size: %d", poolSize)

	var wg sync.WaitGroup
	wg.Add(poolSize)
	for i := 0; i < poolSize; i++ {
		go e.fetch(urlChan, ch, &wg)
	}

	for _, url := range urls {
		urlChan <- url
	}
	close(urlChan)

	wg.Wait()
}

func (e *exporter) updateSlaves() {
	log.Debug("discovering slaves...")

	// This will redirect us to the elected mesos master
	redirectURL := fmt.Sprintf("%s/master/redirect", e.opts.masterURL)
	rReq, err := http.NewRequest("GET", redirectURL, nil)
	if err != nil {
		panic(err)
	}

	tr := http.Transport{
		DisableKeepAlives: true,
	}
	rresp, err := tr.RoundTrip(rReq)
	if err != nil {
		log.Warn(err)
		return
	}
	defer rresp.Body.Close()

	// This will/should return http://master.ip:5050
	masterLoc := rresp.Header.Get("Location")
	if masterLoc == "" {
		log.Warnf("%d response missing Location header", rresp.StatusCode)
		return
	}

	log.Debugf("current elected master at: %s", masterLoc)

	// Find all active slaves
	stateURL := fmt.Sprintf("%s/master/state.json", masterLoc)
	resp, err := http.Get(stateURL)
	if err != nil {
		log.Warn(err)
		return
	}
	defer resp.Body.Close()

	type slave struct {
		Active   bool   `json:"active"`
		Hostname string `json:"hostname"`
		Pid      string `json:"pid"`
	}

	var req struct {
		Slaves []*slave `json:"slaves"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&req); err != nil {
		log.Warnf("failed to deserialize request: %s", err)
		return
	}

	var slaveURLs []string
	for _, slave := range req.Slaves {
		if slave.Active {
			// Extract slave port from pid
			_, port, err := net.SplitHostPort(slave.Pid)
			if err != nil {
				port = "5051"
			}
			url := fmt.Sprintf("http://%s:%s", slave.Hostname, port)

			slaveURLs = append(slaveURLs, url)
		}
	}

	log.Debugf("%d slaves discovered", len(slaveURLs))

	e.slaves.Lock()
	e.slaves.urls = slaveURLs
	e.slaves.Unlock()
}

func runEvery(f func(), interval time.Duration) {
	for _ = range time.NewTicker(interval).C {
		f()
	}
}

func main() {
	flag.Parse()

	opts := &exporterOpts{
		autoDiscover: *autoDiscover,
		localURL:     strings.TrimRight(*localURL, "/"),
		masterURL:    strings.TrimRight(*masterURL, "/"),
	}
	exporter := newMesosExporter(opts)
	prometheus.MustRegister(exporter)

	http.Handle(*metricsPath, prometheus.Handler())
	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "OK")
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, *metricsPath, http.StatusMovedPermanently)
	})

	log.Info("starting mesos_exporter on ", *addr)

	log.Fatal(http.ListenAndServe(*addr, nil))
}
