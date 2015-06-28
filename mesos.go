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

// Represents /metrics/snapshot on mesos-master
type MasterMetrics struct {
	MasterCpusPercent                                     float64 `json:"master/cpus_percent"`
	MasterCpusTotal                                       float64 `json:"master/cpus_total"`
	MasterCpusUsed                                        float64 `json:"master/cpus_used"`
	MasterDiskPercent                                     float64 `json:"master/disk_percent"`
	MasterDiskTotal                                       float64 `json:"master/disk_total"`
	MasterDiskUsed                                        float64 `json:"master/disk_used"`
	MasterDroppedMessages                                 float64 `json:"master/dropped_messages"`
	MasterElected                                         float64 `json:"master/elected"`
	MasterEventQueueDispatches                            float64 `json:"master/event_queue_dispatches"`
	MasterEventQueueHttpRequests                          float64 `json:"master/event_queue_http_requests"`
	MasterEventQueueMessages                              float64 `json:"master/event_queue_messages"`
	MasterFrameworksActive                                float64 `json:"master/frameworks_active"`
	MasterFrameworksConnected                             float64 `json:"master/frameworks_connected"`
	MasterFrameworksDisconnected                          float64 `json:"master/frameworks_disconnected"`
	MasterFrameworksInactive                              float64 `json:"master/frameworks_inactive"`
	MasterInvalidFrameworkToExecutorMessages              float64 `json:"master/invalid_framework_to_executor_messages"`
	MasterInvalidStatusUpdateAcknowledgements             float64 `json:"master/invalid_status_update_acknowledgements"`
	MasterInvalidStatusUpdates                            float64 `json:"master/invalid_status_updates"`
	MasterMemPercent                                      float64 `json:"master/mem_percent"`
	MasterMemTotal                                        float64 `json:"master/mem_total"`
	MasterMemUsed                                         float64 `json:"master/mem_used"`
	MasterMessagesAuthenticate                            float64 `json:"master/messages_authenticate"`
	MasterMessagesDeactivateFramework                     float64 `json:"master/messages_deactivate_framework"`
	MasterMessagesDeclineOffers                           float64 `json:"master/messages_decline_offers"`
	MasterMessagesExitedExecutor                          float64 `json:"master/messages_exited_executor"`
	MasterMessagesFrameworkToExecutor                     float64 `json:"master/messages_framework_to_executor"`
	MasterMessagesKillTask                                float64 `json:"master/messages_kill_task"`
	MasterMessagesLaunchTasks                             float64 `json:"master/messages_launch_tasks"`
	MasterMessagesReconcileTasks                          float64 `json:"master/messages_reconcile_tasks"`
	MasterMessagesRegisterFramework                       float64 `json:"master/messages_register_framework"`
	MasterMessagesRegisterSlave                           float64 `json:"master/messages_register_slave"`
	MasterMessagesReregisterFramework                     float64 `json:"master/messages_reregister_framework"`
	MasterMessagesReregisterSlave                         float64 `json:"master/messages_reregister_slave"`
	MasterMessagesResourceRequest                         float64 `json:"master/messages_resource_request"`
	MasterMessagesReviveOffers                            float64 `json:"master/messages_revive_offers"`
	MasterMessagesStatusUpdate                            float64 `json:"master/messages_status_update"`
	MasterMessagesStatusUpdate_acknowledgement            float64 `json:"master/messages_status_update_acknowledgement"`
	MasterMessagesUnregisterFramework                     float64 `json:"master/messages_unregister_framework"`
	MasterMessagesUnregisterSlave                         float64 `json:"master/messages_unregister_slave"`
	MasterOutstandingOffers                               float64 `json:"master/outstanding_offers"`
	MasterRecoverySlaveRemovals                           float64 `json:"master/recovery_slave_removals"`
	MasterSlaveRegistrations                              float64 `json:"master/slave_registrations"`
	MasterSlaveRemovals                                   float64 `json:"master/slave_removals"`
	MasterSlaveReregistrations                            float64 `json:"master/slave_reregistrations"`
	MasterSlaveShutdownsCanceled                          float64 `json:"master/slave_shutdowns_canceled"`
	MasterSlaveShutdownsScheduled                         float64 `json:"master/slave_shutdowns_scheduled"`
	MasterSlavesActive                                    float64 `json:"master/slaves_active"`
	MasterSlavesConnected                                 float64 `json:"master/slaves_connected"`
	MasterSlavesDisconnected                              float64 `json:"master/slaves_disconnected"`
	MasterSlavesInactive                                  float64 `json:"master/slaves_inactive"`
	MasterTaskFailedSourceSlaveReasonMemoryLimit          float64 `json:"master/task_failed/source_slave/reason_memory_limit"`
	MasterTaskKilledSourceMasterReasonFrameworkRemoved    float64 `json:"master/task_killed/source_master/reason_framework_removed"`
	MasterTaskKilledSourceSlaveReasonExecutorUnregistered float64 `json:"master/task_killed/source_slave/reason_executor_unregistered"`
	MasterTaskLostSourceMasterReasonSlaveDisconnected     float64 `json:"master/task_lost/source_master/reason_slave_disconnected"`
	MasterTaskLostSourceMasterReasonSlaveRemoved          float64 `json:"master/task_lost/source_master/reason_slave_removed"`
	MasterTasksError                                      float64 `json:"master/tasks_error"`
	MasterTasksFailed                                     float64 `json:"master/tasks_failed"`
	MasterTasksFinished                                   float64 `json:"master/tasks_finished"`
	MasterTasksKilled                                     float64 `json:"master/tasks_killed"`
	MasterTasksLost                                       float64 `json:"master/tasks_lost"`
	MasterTasksRunning                                    float64 `json:"master/tasks_running"`
	MasterTasksStaging                                    float64 `json:"master/tasks_staging"`
	MasterTasksStarting                                   float64 `json:"master/tasks_starting"`
	MasterUptimeSecs                                      float64 `json:"master/uptime_secs"`
	MasterValidFrameworkToExecutorMessages                float64 `json:"master/valid_framework_to_executor_messages"`
	MasterValidStatusUpdateAcknowledgements               float64 `json:"master/valid_status_update_acknowledgements"`
	MasterValidStatusUpdates                              float64 `json:"master/valid_status_updates"`
	RegistrarQueuedOperations                             float64 `json:"registrar/queued_operations"`
	RegistrarRegistrySizeBytes                            float64 `json:"registrar/registry_size_bytes"`
	RegistrarStateFetchMs                                 float64 `json:"registrar/state_fetch_ms"`
	RegistrarStateStoreMs                                 float64 `json:"registrar/state_store_ms"`
	RegistrarStateStoreMsCount                            float64 `json:"registrar/state_store_ms/count"`
	RegistrarStateStoreMsMax                              float64 `json:"registrar/state_store_ms/max"`
	RegistrarStateStoreMsMin                              float64 `json:"registrar/state_store_ms/min"`
	RegistrarStateStoreMsP50                              float64 `json:"registrar/state_store_ms/p50"`
	RegistrarStateStoreMsP90                              float64 `json:"registrar/state_store_ms/p90"`
	RegistrarStateStoreMsP95                              float64 `json:"registrar/state_store_ms/p95"`
	RegistrarStateStoreMsP99                              float64 `json:"registrar/state_store_ms/p99"`
	RegistrarStateStoreMsP999                             float64 `json:"registrar/state_store_ms/p999"`
	RegistrarStateStoreMsP9999                            float64 `json:"registrar/state_store_ms/p9999"`
	SystemCpusTotal                                       float64 `json:"system/cpus_total"`
	SystemLoad15min                                       float64 `json:"system/load_15min"`
	SystemLoad1min                                        float64 `json:"system/load_1min"`
	SystemLoad5min                                        float64 `json:"system/load_5min"`
	SystemMemFreeBytes                                    float64 `json:"system/mem_free_bytes"`
	SystemMemTotalBytes                                   float64 `json:"system/mem_total_bytes"`
}
