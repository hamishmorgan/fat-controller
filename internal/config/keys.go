package config

const (
	// Deploy setting keys — used as diff change labels.
	KeyBuilder                 = "builder"
	KeyRepo                    = "repo"
	KeyImage                   = "image"
	KeyBuildCommand            = "build_command"
	KeyDockerfilePath          = "dockerfile_path"
	KeyRootDirectory           = "root_directory"
	KeyWatchPatterns           = "watch_patterns"
	KeyPreDeployCommand        = "pre_deploy_command"
	KeyStartCommand            = "start_command"
	KeyCronSchedule            = "cron_schedule"
	KeyHealthcheckPath         = "healthcheck_path"
	KeyHealthcheckTimeout      = "healthcheck_timeout"
	KeyRestartPolicy           = "restart_policy"
	KeyRestartPolicyMaxRetries = "restart_policy_max_retries"
	KeyDrainingSeconds         = "draining_seconds"
	KeyOverlapSeconds          = "overlap_seconds"
	KeySleepApplication        = "sleep_application"
	KeyNumReplicas             = "num_replicas"
	KeyRegion                  = "region"
	KeyIPv6Egress              = "ipv6_egress"

	// Resource keys.
	KeyVCPUs    = "vcpus"
	KeyMemoryGB = "memory_gb"
)
