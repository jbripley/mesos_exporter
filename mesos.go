package main

// Represents /monitor/statistics.json on mesos-slave
type Monitor struct {
	ExecutorId   string      `json:"executor_id"`
	ExecutorName string      `json:"executor_name"`
	FrameworkId  string      `json:"framework_id"`
	Source       string      `json:"source"`
	Statistics   *Statistics `json:"statistics"`
}

type Statistics struct {
	CpusLimit             float64 `json:"cpus_limit"`
	CpusNrPeriods         float64 `json:"cpus_nr_periods"`
	CpusNrThrottled       float64 `json:"cpus_nr_throttled"`
	CpusSystemTimeSecs    float64 `json:"cpus_system_time_secs"`
	CpusThrottledTimeSecs float64 `json:"cpus_throttled_time_secs"`
	CpusUserTimeSecs      float64 `json:"cpus_user_time_secs"`
	MemAnonBytes          float64 `json:"mem_anon_bytes"`
	MemFileBytes          float64 `json:"mem_file_bytes"`
	MemLimitBytes         float64 `json:"mem_limit_bytes"`
	MemMappedBytes        float64 `json:"mem_mapped_bytes"`
	MemRssBytes           float64 `json:"mem_rss_bytes"`
	NetRxBytes            float64 `json:"net_rx_bytes"`
	NetRxDropped          float64 `json:"net_rx_dropped"`
	NetRxErrors           float64 `json:"net_rx_errors"`
	NetRxPackets          float64 `json:"net_rx_packets"`
	NetTxBytes            float64 `json:"net_tx_bytes"`
	NetTxDropped          float64 `json:"net_tx_dropped"`
	NetTxErrors           float64 `json:"net_tx_errors"`
	NetTxPackets          float64 `json:"net_tx_packets"`
	Timestamp             float64 `json:"timestamp"`
}

// Represents /metrics/snapshot on mesos-slave
type SlaveMetrics struct {
	SlaveCpusPercent              float64 `json:"slave/cpus_percent"`
	SlaveCpusTotal                float64 `json:"slave/cpus_total"`
	SlaveCpusUsed                 float64 `json:"slave/cpus_used"`
	SlaveDiskPercent              float64 `json:"slave/disk_percent"`
	SlaveDiskTotal                float64 `json:"slave/disk_total"`
	SlaveDiskUsed                 float64 `json:"slave/disk_used"`
	SlaveExecutorsRegistering     float64 `json:"slave/executors_registering"`
	SlaveExecutorsRunning         float64 `json:"slave/executors_running"`
	SlaveExecutorsTerminated      float64 `json:"slave/executors_terminated"`
	SlaveExecutorsTerminating     float64 `json:"slave/executors_terminating"`
	SlaveFrameworksActive         float64 `json:"slave/frameworks_active"`
	SlaveInvalidFrameworkMessages float64 `json:"slave/invalid_framework_messages"`
	SlaveInvalidStatusUpdates     float64 `json:"slave/invalid_status_updates"`
	SlaveMemPercent               float64 `json:"slave/mem_percent"`
	SlaveMemTotal                 float64 `json:"slave/mem_total"`
	SlaveMemUsed                  float64 `json:"slave/mem_used"`
	SlaveRecoveryErrors           float64 `json:"slave/recovery_errors"`
	SlaveRegistered               float64 `json:"slave/registered"`
	SlaveTasksFailed              float64 `json:"slave/tasks_failed"`
	SlaveTasksFinished            float64 `json:"slave/tasks_finished"`
	SlaveTasksKilled              float64 `json:"slave/tasks_killed"`
	SlaveTasksLost                float64 `json:"slave/tasks_lost"`
	SlaveTasksRunning             float64 `json:"slave/tasks_running"`
	SlaveTasksStaging             float64 `json:"slave/tasks_staging"`
	SlaveTasksStarting            float64 `json:"slave/tasks_starting"`
	SlaveUptimeSecs               float64 `json:"slave/uptime_secs"`
	SlaveValidFrameworkMessages   float64 `json:"slave/valid_framework_messages"`
	SlaveValidStatusUpdates       float64 `json:"slave/valid_status_updates"`
	SystemCpusTotal               float64 `json:"system/cpus_total"`
	SystemLoad15min               float64 `json:"system/load_15min"`
	SystemLoad1min                float64 `json:"system/load_1min"`
	SystemLoad5min                float64 `json:"system/load_5min"`
	SystemMemFreeBytes            float64 `json:"system/mem_free_bytes"`
	SystemMemTotalBytes           float64 `json:"system/mem_total_bytes"`
}
