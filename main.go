package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/log"
)

const namespace = "mesos"
const concurrentFetch = 100

// Commandline flags.
var (
	addr        = flag.String("web.listen-address", ":9105", "Address to listen on for web interface and telemetry")
	masterAddr  = flag.String("mesos.master-address", "127.0.0.1:5050", "Mesos master address")
	metricsPath = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics")
)

func newMetric(subsys string, labels []string, name string, descr string) *prometheus.Desc {
	fqn := prometheus.BuildFQName(namespace, subsys, name)
	return prometheus.NewDesc(fqn, descr, labels, nil)
}

func newMasterMetric(name string, descr string) *prometheus.Desc {
	return newMetric("master", []string{}, name, descr)
}

func newSlaveMetric(name string, descr string) *prometheus.Desc {
	return newMetric("slave", []string{"instance"}, name, descr)
}

func newSystemMetric(name string, descr string) *prometheus.Desc {
	return newMetric("system", []string{"instance"}, name, descr)
}

func newTaskMetric(name string, descr string) *prometheus.Desc {
	return newMetric("task", []string{"instance", "slave", "framework_id"}, name, descr)
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
	masterURL string
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
	e.slaves.urls = []string{}

	// Update nr. of mesos slaves every 10 minutes
	e.updateSlaves()
	go runEvery(e.updateSlaves, 10*time.Minute)

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

	report := func(mon *Monitor, desc *prometheus.Desc, value float64, kind prometheus.ValueType) {
		metricsChan <- prometheus.MustNewConstMetric(desc,
			kind,
			value,
			mon.Source, host, mon.FrameworkId,
		)
	}

	for _, mon := range stats {
		stats := mon.Statistics
		report(&mon, taskCpuLimitDesc, stats.CpusLimit, prometheus.GaugeValue)
		report(&mon, taskCpuNrPeriodsDesc, stats.CpusNrPeriods, prometheus.CounterValue)
		report(&mon, taskCpuNrThrottledDesc, stats.CpusNrThrottled, prometheus.CounterValue)
		report(&mon, taskCpuSysDesc, stats.CpusSystemTimeSecs, prometheus.GaugeValue)
		report(&mon, taskCpuThrottledDesc, stats.CpusThrottledTimeSecs, prometheus.GaugeValue)
		report(&mon, taskCpuUsrDesc, stats.CpusUserTimeSecs, prometheus.GaugeValue)
		report(&mon, taskDiskLimitBytes, stats.DiskLimitBytes, prometheus.GaugeValue)
		report(&mon, taskDiskUsedBytes, stats.DiskUsedBytes, prometheus.GaugeValue)
		report(&mon, taskMemAnonDesc, stats.MemAnonBytes, prometheus.GaugeValue)
		report(&mon, taskMemFileDesc, stats.MemFileBytes, prometheus.GaugeValue)
		report(&mon, taskMemLimitDesc, stats.MemLimitBytes, prometheus.GaugeValue)
		report(&mon, taskMemMappedDesc, stats.MemMappedBytes, prometheus.GaugeValue)
		report(&mon, taskMemRssDesc, stats.MemRssBytes, prometheus.GaugeValue)
		report(&mon, taskNetRxBytes, stats.NetRxBytes, prometheus.CounterValue)
		report(&mon, taskNetRxDropped, stats.NetRxDropped, prometheus.CounterValue)
		report(&mon, taskNetRxErrors, stats.NetRxErrors, prometheus.CounterValue)
		report(&mon, taskNetRxPackets, stats.NetRxPackets, prometheus.CounterValue)
		report(&mon, taskNetTxBytes, stats.NetTxBytes, prometheus.CounterValue)
		report(&mon, taskNetTxDropped, stats.NetTxDropped, prometheus.CounterValue)
		report(&mon, taskNetTxErrors, stats.NetTxErrors, prometheus.CounterValue)
		report(&mon, taskNetTxPackets, stats.NetTxPackets, prometheus.CounterValue)
	}
}

func (e *exporter) fetchMasterMetrics(metricsChan chan<- prometheus.Metric, wg *sync.WaitGroup) {
	defer wg.Done()

	log.Debugf("Fetching master metrics")

	master, err := e.findMaster()
	if err != nil {
		log.Warnf("Error finding elected master: %s", err)
		return
	}

	url := fmt.Sprintf("http://%s/metrics/snapshot", master)
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

	report := func(desc *prometheus.Desc, value float64, kind prometheus.ValueType) {
		metricsChan <- prometheus.MustNewConstMetric(desc, kind, value)
	}

	report(masterCpusPercent, metrics.MasterCpusPercent, prometheus.GaugeValue)
	report(masterCpusTotal, metrics.MasterCpusTotal, prometheus.GaugeValue)
	report(masterCpusUsed, metrics.MasterCpusUsed, prometheus.GaugeValue)
	report(masterDiskPercent, metrics.MasterDiskPercent, prometheus.GaugeValue)
	report(masterDiskTotal, metrics.MasterDiskTotal, prometheus.GaugeValue)
	report(masterDiskUsed, metrics.MasterDiskUsed, prometheus.GaugeValue)
	report(masterDroppedMessages, metrics.MasterDroppedMessages, prometheus.CounterValue)
	report(masterElected, metrics.MasterElected, prometheus.GaugeValue)
	report(masterEventQueueDispatches, metrics.MasterEventQueueDispatches, prometheus.GaugeValue)
	report(masterEventQueueHttpRequests, metrics.MasterEventQueueHttpRequests, prometheus.GaugeValue)
	report(masterEventQueueMessages, metrics.MasterEventQueueMessages, prometheus.GaugeValue)
	report(masterFrameworksActive, metrics.MasterFrameworksActive, prometheus.GaugeValue)
	report(masterFrameworksConnected, metrics.MasterFrameworksConnected, prometheus.GaugeValue)
	report(masterFrameworksDisconnected, metrics.MasterFrameworksDisconnected, prometheus.GaugeValue)
	report(masterFrameworksInactive, metrics.MasterFrameworksInactive, prometheus.GaugeValue)
	report(masterInvalidFrameworkToExecutorMessages, metrics.MasterInvalidFrameworkToExecutorMessages, prometheus.CounterValue)
	report(masterInvalidStatusUpdateAcknowledgements, metrics.MasterInvalidStatusUpdateAcknowledgements, prometheus.CounterValue)
	report(masterInvalidStatusUpdates, metrics.MasterInvalidStatusUpdates, prometheus.CounterValue)
	report(masterMemPercent, metrics.MasterMemPercent, prometheus.GaugeValue)
	report(masterMemTotal, metrics.MasterMemTotal, prometheus.GaugeValue)
	report(masterMemUsed, metrics.MasterMemUsed, prometheus.GaugeValue)
	report(masterMessagesAuthenticate, metrics.MasterMessagesAuthenticate, prometheus.CounterValue)
	report(masterMessagesDeactivateFramework, metrics.MasterMessagesDeactivateFramework, prometheus.CounterValue)
	report(masterMessagesDeclineOffers, metrics.MasterMessagesDeclineOffers, prometheus.CounterValue)
	report(masterMessagesExitedExecutor, metrics.MasterMessagesExitedExecutor, prometheus.CounterValue)
	report(masterMessagesFrameworkToExecutor, metrics.MasterMessagesFrameworkToExecutor, prometheus.CounterValue)
	report(masterMessagesKillTask, metrics.MasterMessagesKillTask, prometheus.CounterValue)
	report(masterMessagesLaunchTasks, metrics.MasterMessagesLaunchTasks, prometheus.CounterValue)
	report(masterMessagesReconcileTasks, metrics.MasterMessagesReconcileTasks, prometheus.CounterValue)
	report(masterMessagesRegisterFramework, metrics.MasterMessagesRegisterFramework, prometheus.CounterValue)
	report(masterMessagesRegisterSlave, metrics.MasterMessagesRegisterSlave, prometheus.CounterValue)
	report(masterMessagesReregisterFramework, metrics.MasterMessagesReregisterFramework, prometheus.CounterValue)
	report(masterMessagesReregisterSlave, metrics.MasterMessagesReregisterSlave, prometheus.CounterValue)
	report(masterMessagesResourceRequest, metrics.MasterMessagesResourceRequest, prometheus.CounterValue)
	report(masterMessagesReviveOffers, metrics.MasterMessagesReviveOffers, prometheus.CounterValue)
	report(masterMessagesStatusUpdate, metrics.MasterMessagesStatusUpdate, prometheus.CounterValue)
	report(masterMessagesStatusUpdate_acknowledgement, metrics.MasterMessagesStatusUpdate_acknowledgement, prometheus.CounterValue)
	report(masterMessagesUnregisterFramework, metrics.MasterMessagesUnregisterFramework, prometheus.CounterValue)
	report(masterMessagesUnregisterSlave, metrics.MasterMessagesUnregisterSlave, prometheus.CounterValue)
	report(masterOutstandingOffers, metrics.MasterOutstandingOffers, prometheus.GaugeValue)
	report(masterRecoverySlaveRemovals, metrics.MasterRecoverySlaveRemovals, prometheus.GaugeValue)
	report(masterSlaveRegistrations, metrics.MasterSlaveRegistrations, prometheus.CounterValue)
	report(masterSlaveRemovals, metrics.MasterSlaveRemovals, prometheus.CounterValue)
	report(masterSlaveReregistrations, metrics.MasterSlaveReregistrations, prometheus.CounterValue)
	report(masterSlaveShutdownsCanceled, metrics.MasterSlaveShutdownsCanceled, prometheus.CounterValue)
	report(masterSlaveShutdownsScheduled, metrics.MasterSlaveShutdownsScheduled, prometheus.CounterValue)
	report(masterSlavesActive, metrics.MasterSlavesActive, prometheus.GaugeValue)
	report(masterSlavesConnected, metrics.MasterSlavesConnected, prometheus.GaugeValue)
	report(masterSlavesDisconnected, metrics.MasterSlavesDisconnected, prometheus.GaugeValue)
	report(masterSlavesInactive, metrics.MasterSlavesInactive, prometheus.GaugeValue)
	report(masterTaskFailedSourceSlaveReasonMemoryLimit, metrics.MasterTaskFailedSourceSlaveReasonMemoryLimit, prometheus.CounterValue)
	report(masterTaskKilledSourceMasterReasonFrameworkRemoved, metrics.MasterTaskKilledSourceMasterReasonFrameworkRemoved, prometheus.CounterValue)
	report(masterTaskKilledSourceSlaveReasonExecutorUnregistered, metrics.MasterTaskKilledSourceSlaveReasonExecutorUnregistered, prometheus.CounterValue)
	report(masterTaskLostSourceMasterReasonSlaveDisconnected, metrics.MasterTaskLostSourceMasterReasonSlaveDisconnected, prometheus.CounterValue)
	report(masterTaskLostSourceMasterReasonSlaveRemoved, metrics.MasterTaskLostSourceMasterReasonSlaveRemoved, prometheus.CounterValue)
	report(masterTasksError, metrics.MasterTasksError, prometheus.CounterValue)
	report(masterTasksFailed, metrics.MasterTasksFailed, prometheus.CounterValue)
	report(masterTasksFinished, metrics.MasterTasksFinished, prometheus.CounterValue)
	report(masterTasksKilled, metrics.MasterTasksKilled, prometheus.CounterValue)
	report(masterTasksLost, metrics.MasterTasksLost, prometheus.CounterValue)
	report(masterTasksRunning, metrics.MasterTasksRunning, prometheus.GaugeValue)
	report(masterTasksStaging, metrics.MasterTasksStaging, prometheus.GaugeValue)
	report(masterTasksStarting, metrics.MasterTasksStarting, prometheus.GaugeValue)
	report(masterUptimeSecs, metrics.MasterUptimeSecs, prometheus.GaugeValue)
	report(masterValidFrameworkToExecutorMessages, metrics.MasterValidFrameworkToExecutorMessages, prometheus.CounterValue)
	report(masterValidStatusUpdateAcknowledgements, metrics.MasterValidStatusUpdateAcknowledgements, prometheus.CounterValue)
	report(masterValidStatusUpdates, metrics.MasterValidStatusUpdates, prometheus.CounterValue)

	report(registrarQueuedOperations, metrics.RegistrarQueuedOperations, prometheus.GaugeValue)
	report(registrarRegistrySizeBytes, metrics.RegistrarRegistrySizeBytes, prometheus.GaugeValue)
	report(registrarStateFetchMs, metrics.RegistrarStateFetchMs, prometheus.GaugeValue)
	report(registrarStateStoreMs, metrics.RegistrarStateStoreMs, prometheus.GaugeValue)
	report(registrarStateStoreMsCount, metrics.RegistrarStateStoreMsCount, prometheus.GaugeValue)
	report(registrarStateStoreMsMax, metrics.RegistrarStateStoreMsMax, prometheus.GaugeValue)
	report(registrarStateStoreMsMin, metrics.RegistrarStateStoreMsMin, prometheus.GaugeValue)
	report(registrarStateStoreMsP50, metrics.RegistrarStateStoreMsP50, prometheus.GaugeValue)
	report(registrarStateStoreMsP90, metrics.RegistrarStateStoreMsP90, prometheus.GaugeValue)
	report(registrarStateStoreMsP95, metrics.RegistrarStateStoreMsP95, prometheus.GaugeValue)
	report(registrarStateStoreMsP99, metrics.RegistrarStateStoreMsP99, prometheus.GaugeValue)
	report(registrarStateStoreMsP999, metrics.RegistrarStateStoreMsP999, prometheus.GaugeValue)
	report(registrarStateStoreMsP9999, metrics.RegistrarStateStoreMsP9999, prometheus.GaugeValue)

	report(masterSystemCpusTotal, metrics.SystemCpusTotal, prometheus.GaugeValue)
	report(masterSystemLoad15min, metrics.SystemLoad15min, prometheus.GaugeValue)
	report(masterSystemLoad1min, metrics.SystemLoad1min, prometheus.GaugeValue)
	report(masterSystemLoad5min, metrics.SystemLoad5min, prometheus.GaugeValue)
	report(masterSystemMemFreeBytes, metrics.SystemMemFreeBytes, prometheus.GaugeValue)
	report(masterSystemMemTotalBytes, metrics.SystemMemTotalBytes, prometheus.GaugeValue)
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

	report := func(desc *prometheus.Desc, value float64, kind prometheus.ValueType) {
		metricsChan <- prometheus.MustNewConstMetric(desc, kind, value, host)
	}

	report(slaveCpusPercent, metrics.SlaveCpusPercent, prometheus.GaugeValue)
	report(slaveCpusTotal, metrics.SlaveCpusTotal, prometheus.GaugeValue)
	report(slaveCpusUsed, metrics.SlaveCpusUsed, prometheus.GaugeValue)
	report(slaveDiskPercent, metrics.SlaveDiskPercent, prometheus.GaugeValue)
	report(slaveDiskTotal, metrics.SlaveDiskTotal, prometheus.GaugeValue)
	report(slaveDiskUsed, metrics.SlaveDiskUsed, prometheus.GaugeValue)
	report(slaveExecutorsRegistering, metrics.SlaveExecutorsRegistering, prometheus.GaugeValue)
	report(slaveExecutorsRunning, metrics.SlaveExecutorsRunning, prometheus.GaugeValue)
	report(slaveExecutorsTerminated, metrics.SlaveExecutorsTerminated, prometheus.CounterValue)
	report(slaveExecutorsTerminating, metrics.SlaveExecutorsTerminating, prometheus.GaugeValue)
	report(slaveFrameworksActive, metrics.SlaveFrameworksActive, prometheus.GaugeValue)
	report(slaveInvalidFrameworkMessages, metrics.SlaveInvalidFrameworkMessages, prometheus.CounterValue)
	report(slaveInvalidStatusUpdates, metrics.SlaveInvalidStatusUpdates, prometheus.CounterValue)
	report(slaveMemPercent, metrics.SlaveMemPercent, prometheus.GaugeValue)
	report(slaveMemTotal, metrics.SlaveMemTotal, prometheus.GaugeValue)
	report(slaveMemUsed, metrics.SlaveMemUsed, prometheus.GaugeValue)
	report(slaveRecoveryErrors, metrics.SlaveRecoveryErrors, prometheus.CounterValue)
	report(slaveRegistered, metrics.SlaveRegistered, prometheus.GaugeValue)
	report(slaveTasksFailed, metrics.SlaveTasksFailed, prometheus.CounterValue)
	report(slaveTasksFinished, metrics.SlaveTasksFinished, prometheus.CounterValue)
	report(slaveTasksKilled, metrics.SlaveTasksKilled, prometheus.CounterValue)
	report(slaveTasksLost, metrics.SlaveTasksLost, prometheus.CounterValue)
	report(slaveTasksRunning, metrics.SlaveTasksRunning, prometheus.GaugeValue)
	report(slaveTasksStaging, metrics.SlaveTasksStaging, prometheus.GaugeValue)
	report(slaveTasksStarting, metrics.SlaveTasksStarting, prometheus.GaugeValue)
	report(slaveUptimeSecs, metrics.SlaveUptimeSecs, prometheus.GaugeValue)
	report(slaveValidFrameworkMessages, metrics.SlaveValidFrameworkMessages, prometheus.CounterValue)
	report(slaveValidStatusUpdates, metrics.SlaveValidStatusUpdates, prometheus.CounterValue)
	report(sysCpusTotal, metrics.SystemCpusTotal, prometheus.GaugeValue)
	report(sysLoad15min, metrics.SystemLoad15min, prometheus.GaugeValue)
	report(sysLoad1min, metrics.SystemLoad1min, prometheus.GaugeValue)
	report(sysLoad5min, metrics.SystemLoad5min, prometheus.GaugeValue)
	report(sysMemFreeBytes, metrics.SystemMemFreeBytes, prometheus.GaugeValue)
	report(sysMemTotalBytes, metrics.SystemMemTotalBytes, prometheus.GaugeValue)
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

	// This will return //master.ip:5050 mesos >0.27
	masterLoc := rresp.Header.Get("Location")
	if masterLoc == "" {
		log.Warnf("%d response missing Location header", rresp.StatusCode)
		// FIXME: What to do here?
	}
	u, err := url.Parse(masterLoc)
	if err != nil {
		log.Fatal(err)
	}
	log.Debugf("current elected master at: %s", u.Host)
	return u.Host, nil
}

func (e *exporter) updateSlaves() {
	log.Debug("discovering slaves...")

	master, err := e.findMaster()
	if err != nil {
		log.Warnf("Error finding elected master: %s", err)
		return
	}

	// Find all active slaves
	stateURL := fmt.Sprintf("http://%s/master/state.json", master)
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
		masterURL: fmt.Sprintf("http://%s", *masterAddr),
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
