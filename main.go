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
const defaultMasterURL = "http://mesos-master.example.com:5050"

// Commandline flags.
var (
	addr         = flag.String("web.listen-address", ":9105", "Address to listen on for web interface and telemetry")
	autoDiscover = flag.Bool("exporter.discovery", false, "Discover all Mesos slaves")
	localURL     = flag.String("exporter.local-url", "http://127.0.0.1:5051", "URL to the local Mesos slave")
	masterURL    = flag.String("exporter.discovery.master-url", defaultMasterURL, "Mesos master URL")
	metricsPath  = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics")
)

func newMetric(subsys string, labels []string, name string, descr string) *prometheus.Desc {
	fqn := prometheus.BuildFQName(namespace, subsys, name)
	return prometheus.NewDesc(fqn, descr, labels, nil)
}

func newMasterMetric(name string, descr string) *prometheus.Desc {
	return newMetric("master", []string{}, name, descr)
}

func newSlaveMetric(name string, descr string) *prometheus.Desc {
	return newMetric("slave", []string{"slave"}, name, descr)
}

func newSystemMetric(name string, descr string) *prometheus.Desc {
	return newMetric("system", []string{"slave"}, name, descr)
}

func newTaskMetric(name string, descr string) *prometheus.Desc {
	return newMetric("task", []string{"task", "slave", "framework_id"}, name, descr)
}

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

func (e *exporter) fetchTaskMetrics(host string, port string, metricsChan chan<- prometheus.Metric, wg *sync.WaitGroup) {
	defer wg.Done()

	log.Debugf("Fetching task metrics for slave %s:%s", host, port)

	url := fmt.Sprintf("http://%s:%s/monitor/statistics.json", host, port)
	resp, err := httpClient.Get(url)
	if err != nil {
		log.Warn(err)
		e.errors.WithLabelValues(host).Inc()
		return
	}
	defer resp.Body.Close()

	var stats []Monitor
	if err = json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		log.Warn("failed to deserialize response: ", err)
		e.errors.WithLabelValues(host).Inc()
		return
	}

	report := func(mon *Monitor, desc *prometheus.Desc, value float64) {
		metricsChan <- prometheus.MustNewConstMetric(desc,
			prometheus.GaugeValue,
			value,
			mon.Source, host, mon.FrameworkId,
		)
	}

	for _, mon := range stats {
		stats := mon.Statistics
		report(&mon, taskCpuLimitDesc, stats.CpusLimit)
		report(&mon, taskCpuNrPeriodsDesc, stats.CpusNrPeriods)
		report(&mon, taskCpuNrThrottledDesc, stats.CpusNrThrottled)
		report(&mon, taskCpuSysDesc, stats.CpusSystemTimeSecs)
		report(&mon, taskCpuThrottledDesc, stats.CpusThrottledTimeSecs)
		report(&mon, taskCpuUsrDesc, stats.CpusUserTimeSecs)
		report(&mon, taskMemAnonDesc, stats.MemAnonBytes)
		report(&mon, taskMemFileDesc, stats.MemFileBytes)
		report(&mon, taskMemLimitDesc, stats.MemLimitBytes)
		report(&mon, taskMemMappedDesc, stats.MemMappedBytes)
		report(&mon, taskMemRssDesc, stats.MemRssBytes)
		report(&mon, taskNetRxBytes, stats.NetRxBytes)
		report(&mon, taskNetRxDropped, stats.NetRxDropped)
		report(&mon, taskNetRxErrors, stats.NetRxErrors)
		report(&mon, taskNetRxPackets, stats.NetRxPackets)
		report(&mon, taskNetTxBytes, stats.NetTxBytes)
		report(&mon, taskNetTxDropped, stats.NetTxDropped)
		report(&mon, taskNetTxErrors, stats.NetTxErrors)
		report(&mon, taskNetTxPackets, stats.NetTxPackets)
	}
}

func (e *exporter) fetchMasterMetrics(metricsChan chan<- prometheus.Metric, wg *sync.WaitGroup) {
	defer wg.Done()

	if e.opts.masterURL == defaultMasterURL {
		log.Info("Master metrics is only enabled when using autodiscovery.")
		return
	}

	log.Debugf("Fetching master metrics")

	masterUrl, err := e.findMaster()
	if err != nil {
		log.Warnf("Error finding elected master: %s", err)
		return
	}

	url := fmt.Sprintf("%s/metrics/snapshot", masterUrl)
	resp, err := httpClient.Get(url)
	if err != nil {
		log.Warn(err)
		return
	}
	defer resp.Body.Close()

	var metrics MasterMetrics
	if err = json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
		log.Warn("failed to deserialize response: ", err)
		return
	}

	report := func(desc *prometheus.Desc, value float64) {
		metricsChan <- prometheus.MustNewConstMetric(
			desc,
			prometheus.GaugeValue,
			value,
		)
	}

	report(masterCpusPercent, metrics.MasterCpusPercent)
	report(masterCpusTotal, metrics.MasterCpusTotal)
	report(masterCpusUsed, metrics.MasterCpusUsed)
	report(masterDiskPercent, metrics.MasterDiskPercent)
	report(masterDiskTotal, metrics.MasterDiskTotal)
	report(masterDiskUsed, metrics.MasterDiskUsed)
	report(masterDroppedMessages, metrics.MasterDroppedMessages)
	report(masterElected, metrics.MasterElected)
	report(masterEventQueueDispatches, metrics.MasterEventQueueDispatches)
	report(masterEventQueueHttpRequests, metrics.MasterEventQueueHttpRequests)
	report(masterEventQueueMessages, metrics.MasterEventQueueMessages)
	report(masterFrameworksActive, metrics.MasterFrameworksActive)
	report(masterFrameworksConnected, metrics.MasterFrameworksConnected)
	report(masterFrameworksDisconnected, metrics.MasterFrameworksDisconnected)
	report(masterFrameworksInactive, metrics.MasterFrameworksInactive)
	report(masterInvalidFrameworkToExecutorMessages, metrics.MasterInvalidFrameworkToExecutorMessages)
	report(masterInvalidStatusUpdateAcknowledgements, metrics.MasterInvalidStatusUpdateAcknowledgements)
	report(masterInvalidStatusUpdates, metrics.MasterInvalidStatusUpdates)
	report(masterMemPercent, metrics.MasterMemPercent)
	report(masterMemTotal, metrics.MasterMemTotal)
	report(masterMemUsed, metrics.MasterMemUsed)
	report(masterMessagesAuthenticate, metrics.MasterMessagesAuthenticate)
	report(masterMessagesDeactivateFramework, metrics.MasterMessagesDeactivateFramework)
	report(masterMessagesDeclineOffers, metrics.MasterMessagesDeclineOffers)
	report(masterMessagesExitedExecutor, metrics.MasterMessagesExitedExecutor)
	report(masterMessagesFrameworkToExecutor, metrics.MasterMessagesFrameworkToExecutor)
	report(masterMessagesKillTask, metrics.MasterMessagesKillTask)
	report(masterMessagesLaunchTasks, metrics.MasterMessagesLaunchTasks)
	report(masterMessagesReconcileTasks, metrics.MasterMessagesReconcileTasks)
	report(masterMessagesRegisterFramework, metrics.MasterMessagesRegisterFramework)
	report(masterMessagesRegisterSlave, metrics.MasterMessagesRegisterSlave)
	report(masterMessagesReregisterFramework, metrics.MasterMessagesReregisterFramework)
	report(masterMessagesReregisterSlave, metrics.MasterMessagesReregisterSlave)
	report(masterMessagesResourceRequest, metrics.MasterMessagesResourceRequest)
	report(masterMessagesReviveOffers, metrics.MasterMessagesReviveOffers)
	report(masterMessagesStatusUpdate, metrics.MasterMessagesStatusUpdate)
	report(masterMessagesStatusUpdate_acknowledgement, metrics.MasterMessagesStatusUpdate_acknowledgement)
	report(masterMessagesUnregisterFramework, metrics.MasterMessagesUnregisterFramework)
	report(masterMessagesUnregisterSlave, metrics.MasterMessagesUnregisterSlave)
	report(masterOutstandingOffers, metrics.MasterOutstandingOffers)
	report(masterRecoverySlaveRemovals, metrics.MasterRecoverySlaveRemovals)
	report(masterSlaveRegistrations, metrics.MasterSlaveRegistrations)
	report(masterSlaveRemovals, metrics.MasterSlaveRemovals)
	report(masterSlaveReregistrations, metrics.MasterSlaveReregistrations)
	report(masterSlaveShutdownsCanceled, metrics.MasterSlaveShutdownsCanceled)
	report(masterSlaveShutdownsScheduled, metrics.MasterSlaveShutdownsScheduled)
	report(masterSlavesActive, metrics.MasterSlavesActive)
	report(masterSlavesConnected, metrics.MasterSlavesConnected)
	report(masterSlavesDisconnected, metrics.MasterSlavesDisconnected)
	report(masterSlavesInactive, metrics.MasterSlavesInactive)
	report(masterTaskFailedSourceSlaveReasonMemoryLimit, metrics.MasterTaskFailedSourceSlaveReasonMemoryLimit)
	report(masterTaskKilledSourceMasterReasonFrameworkRemoved, metrics.MasterTaskKilledSourceMasterReasonFrameworkRemoved)
	report(masterTaskKilledSourceSlaveReasonExecutorUnregistered, metrics.MasterTaskKilledSourceSlaveReasonExecutorUnregistered)
	report(masterTaskLostSourceMasterReasonSlaveDisconnected, metrics.MasterTaskLostSourceMasterReasonSlaveDisconnected)
	report(masterTaskLostSourceMasterReasonSlaveRemoved, metrics.MasterTaskLostSourceMasterReasonSlaveRemoved)
	report(masterTasksError, metrics.MasterTasksError)
	report(masterTasksFailed, metrics.MasterTasksFailed)
	report(masterTasksFinished, metrics.MasterTasksFinished)
	report(masterTasksKilled, metrics.MasterTasksKilled)
	report(masterTasksLost, metrics.MasterTasksLost)
	report(masterTasksRunning, metrics.MasterTasksRunning)
	report(masterTasksStaging, metrics.MasterTasksStaging)
	report(masterTasksStarting, metrics.MasterTasksStarting)
	report(masterUptimeSecs, metrics.MasterUptimeSecs)
	report(masterValidFrameworkToExecutorMessages, metrics.MasterValidFrameworkToExecutorMessages)
	report(masterValidStatusUpdateAcknowledgements, metrics.MasterValidStatusUpdateAcknowledgements)
	report(masterValidStatusUpdates, metrics.MasterValidStatusUpdates)

	report(registrarQueuedOperations, metrics.RegistrarQueuedOperations)
	report(registrarRegistrySizeBytes, metrics.RegistrarRegistrySizeBytes)
	report(registrarStateFetchMs, metrics.RegistrarStateFetchMs)
	report(registrarStateStoreMs, metrics.RegistrarStateStoreMs)
	report(registrarStateStoreMsCount, metrics.RegistrarStateStoreMsCount)
	report(registrarStateStoreMsMax, metrics.RegistrarStateStoreMsMax)
	report(registrarStateStoreMsMin, metrics.RegistrarStateStoreMsMin)
	report(registrarStateStoreMsP50, metrics.RegistrarStateStoreMsP50)
	report(registrarStateStoreMsP90, metrics.RegistrarStateStoreMsP90)
	report(registrarStateStoreMsP95, metrics.RegistrarStateStoreMsP95)
	report(registrarStateStoreMsP99, metrics.RegistrarStateStoreMsP99)
	report(registrarStateStoreMsP999, metrics.RegistrarStateStoreMsP999)
	report(registrarStateStoreMsP9999, metrics.RegistrarStateStoreMsP9999)

	report(masterSystemCpusTotal, metrics.SystemCpusTotal)
	report(masterSystemLoad15min, metrics.SystemLoad15min)
	report(masterSystemLoad1min, metrics.SystemLoad1min)
	report(masterSystemLoad5min, metrics.SystemLoad5min)
	report(masterSystemMemFreeBytes, metrics.SystemMemFreeBytes)
	report(masterSystemMemTotalBytes, metrics.SystemMemTotalBytes)
}

func (e *exporter) fetchSlaveMetrics(host string, port string, metricsChan chan<- prometheus.Metric, wg *sync.WaitGroup) {
	defer wg.Done()

	log.Debugf("Fetching slave metrics for %s:%s", host, port)

	url := fmt.Sprintf("http://%s:%s/metrics/snapshot", host, port)
	resp, err := httpClient.Get(url)
	if err != nil {
		log.Warn(err)
		e.errors.WithLabelValues(host).Inc()
		return
	}
	defer resp.Body.Close()

	var metrics SlaveMetrics
	if err = json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
		log.Warn("failed to deserialize response: ", err)
		e.errors.WithLabelValues(host).Inc()
		return
	}

	report := func(desc *prometheus.Desc, value float64) {
		metricsChan <- prometheus.MustNewConstMetric(
			desc,
			prometheus.GaugeValue,
			value,
			host,
		)
	}

	report(slaveCpusPercent, metrics.SlaveCpusPercent)
	report(slaveCpusTotal, metrics.SlaveCpusTotal)
	report(slaveCpusUsed, metrics.SlaveCpusUsed)
	report(slaveDiskPercent, metrics.SlaveDiskPercent)
	report(slaveDiskTotal, metrics.SlaveDiskTotal)
	report(slaveDiskUsed, metrics.SlaveDiskUsed)
	report(slaveExecutorsRegistering, metrics.SlaveExecutorsRegistering)
	report(slaveExecutorsRunning, metrics.SlaveExecutorsRunning)
	report(slaveExecutorsTerminated, metrics.SlaveExecutorsTerminated)
	report(slaveExecutorsTerminating, metrics.SlaveExecutorsTerminating)
	report(slaveFrameworksActive, metrics.SlaveFrameworksActive)
	report(slaveInvalidFrameworkMessages, metrics.SlaveInvalidFrameworkMessages)
	report(slaveInvalidStatusUpdates, metrics.SlaveInvalidStatusUpdates)
	report(slaveMemPercent, metrics.SlaveMemPercent)
	report(slaveMemTotal, metrics.SlaveMemTotal)
	report(slaveMemUsed, metrics.SlaveMemUsed)
	report(slaveRecoveryErrors, metrics.SlaveRecoveryErrors)
	report(slaveRegistered, metrics.SlaveRegistered)
	report(slaveTasksFailed, metrics.SlaveTasksFailed)
	report(slaveTasksFinished, metrics.SlaveTasksFinished)
	report(slaveTasksKilled, metrics.SlaveTasksKilled)
	report(slaveTasksLost, metrics.SlaveTasksLost)
	report(slaveTasksRunning, metrics.SlaveTasksRunning)
	report(slaveTasksStaging, metrics.SlaveTasksStaging)
	report(slaveTasksStarting, metrics.SlaveTasksStarting)
	report(slaveUptimeSecs, metrics.SlaveUptimeSecs)
	report(slaveValidFrameworkMessages, metrics.SlaveValidFrameworkMessages)
	report(slaveValidStatusUpdates, metrics.SlaveValidStatusUpdates)
	report(sysCpusTotal, metrics.SystemCpusTotal)
	report(sysLoad15min, metrics.SystemLoad15min)
	report(sysLoad1min, metrics.SystemLoad1min)
	report(sysLoad5min, metrics.SystemLoad5min)
	report(sysMemFreeBytes, metrics.SystemMemFreeBytes)
	report(sysMemTotalBytes, metrics.SystemMemTotalBytes)
}

func (e *exporter) fetch(urlChan <-chan string, metricsChan chan<- prometheus.Metric, wg *sync.WaitGroup) {
	defer wg.Done()

	for u := range urlChan {
		u, err := url.Parse(u)
		if err != nil {
			log.Warn("could not parse slave URL: ", err)
			continue
		}

		host, port, err := net.SplitHostPort(u.Host)
		if err != nil {
			log.Warn("could not parse network address: ", err)
			continue
		}

		wg.Add(2)
		go e.fetchTaskMetrics(host, port, metricsChan, wg)
		go e.fetchSlaveMetrics(host, port, metricsChan, wg)
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

	// Scrape master
	wg.Add(1)
	go e.fetchMasterMetrics(ch, &wg)

	wg.Wait()
}

func (e *exporter) findMaster() (string, error) {
	// This will redirect us to the elected mesos master
	redirectURL := fmt.Sprintf("%s/master/redirect", e.opts.masterURL)
	rReq, err := http.NewRequest("GET", redirectURL, nil)
	if err != nil {
		return "", err
	}

	tr := http.Transport{
		DisableKeepAlives: true,
	}
	rresp, err := tr.RoundTrip(rReq)
	if err != nil {
		return "", err
	}
	defer rresp.Body.Close()

	// This will/should return http://master.ip:5050
	masterLoc := rresp.Header.Get("Location")
	if masterLoc == "" {
		log.Warnf("%d response missing Location header", rresp.StatusCode)
		// FIXME: What to do here?
	}

	log.Debugf("current elected master at: %s", masterLoc)
	return masterLoc, nil
}

func (e *exporter) updateSlaves() {
	log.Debug("discovering slaves...")

	masterUrl, err := e.findMaster()
	if err != nil {
		log.Warnf("Error finding elected master: %s", err)
		return
	}

	// Find all active slaves
	stateURL := fmt.Sprintf("%s/master/state.json", masterUrl)
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
