package agent

import (
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/toni-moreno/syncflux/pkg/config"
)

var (
	// Version is the app X.Y.Z version
	Version string
	// Commit is the git commit sha1
	Commit string
	// Branch is the git branch
	Branch string
	// BuildStamp is the build timestamp
	BuildStamp string
)

// RInfo contains the agent's release and version information.
type RInfo struct {
	InstanceID string
	Version    string
	Commit     string
	Branch     string
	BuildStamp string
}

// GetRInfo returns the agent release information.
func GetRInfo() *RInfo {
	info := &RInfo{
		InstanceID: MainConfig.General.InstanceID,
		Version:    Version,
		Commit:     Commit,
		Branch:     Branch,
		BuildStamp: BuildStamp,
	}
	return info
}

var (

	// MainConfig contains the global configuration
	MainConfig config.Config

	log *logrus.Logger
	// reloadMutex guards the reloadProcess flag
	reloadMutex   sync.Mutex
	reloadProcess bool
	// mutex guards the runtime devices map access
	mutex sync.RWMutex

	processWg sync.WaitGroup

	Cluster *HACluster
)

// SetLogger sets the current log output.
func SetLogger(l *logrus.Logger) {
	log = l
}

func initCluster() *HACluster {
	log.Infof("Initializing cluster")

	var MDB *InfluxMonitor
	var SDB *InfluxMonitor

	for {
		slaveFound := false
		masterAlive := true
		masterFound := false
		slaveAlive := true

		for _, idb := range MainConfig.InfluxArray {
			if idb.Name == MainConfig.General.MasterDB {
				masterFound = true
				log.Infof("Found MasterDB in config File %+v", idb)
				MDB = &InfluxMonitor{cfg: idb, CheckInterval: MainConfig.General.CheckInterval}

				_, _, _, err := MDB.InitPing()
				if err != nil {
					masterAlive = false
					log.Errorf("MasterDB has  problems :%s", err)
				}

			}
			if idb.Name == MainConfig.General.SlaveDB {
				slaveFound = true
				log.Infof("Found SlaveDB in config File %+v", idb)
				SDB = &InfluxMonitor{cfg: idb, CheckInterval: MainConfig.General.CheckInterval}

				_, _, _, err := SDB.InitPing()
				if err != nil {
					slaveAlive = false
					log.Errorf("SlaveDB has  problems :%s", err)
				}
			}

		}

		if slaveFound && masterFound && masterAlive && slaveAlive {
			return &HACluster{
				Master:        MDB,
				Slave:         SDB,
				CheckInterval: MainConfig.General.MinSyncInterval,
				ClusterState:  "OK",
				SlaveStateOK:  true,
				SlaveLastOK:   time.Now(),
				MasterStateOK: true,
				MasterLastOK:  time.Now(),
			}

		} else {
			if !slaveFound {
				log.Errorf("No Slave DB  found, please check config and restart the process")
			}
			if !masterFound {
				log.Errorf("No Master DB found, please check config and restart the process")
			}
			if !masterAlive {
				log.Errorf("Master DB is not runing I should wait until both up to begin to chek sync status")
			}
			if !slaveAlive {
				log.Errorf("Slave DB is not runing I should wait until both up to begin to chek sync status")
			}
		}
		time.Sleep(MainConfig.General.MonitorRetryInterval)
	}
}

func Copy(dbs string, start time.Time, end time.Time) {

	Cluster = initCluster()

	schema, _ := Cluster.GetSchema()
	Cluster.ReplicateData(schema, start, end)

}

func HAMonitorStart() {

	Cluster = initCluster()

	schema, _ := Cluster.GetSchema()

	switch MainConfig.General.InitialReplication {
	case "schema":
		log.Info("Replicating DB Schema from Master to Slave")
		Cluster.ReplicateSchema(schema)
	case "data":
		log.Info("Replicating DATA Schema from Master to Slave")
		Cluster.ReplicateDataFull(schema)
	case "both":
		log.Info("Replicating DB Schema from Master to Slave")
		Cluster.ReplicateSchema(schema)
		log.Info("Replicating DATA Schema from Master to Slave")
		Cluster.ReplicateDataFull(schema)
	case "none":
		log.Info("No replication done")
	default:
		log.Errorf("Unknown replication config %s", MainConfig.General.InitialReplication)
	}

	Cluster.Master.StartMonitor(&processWg)
	Cluster.Slave.StartMonitor(&processWg)
	time.Sleep(MainConfig.General.CheckInterval)
	Cluster.SuperVisor(&processWg)

}

// End stops all devices polling.
func End() (time.Duration, error) {

	start := time.Now()
	log.Infof("END: begin device Gather processes stop... at %s", start.String())

	// wait until Done
	processWg.Wait()

	log.Infof("END: Finished from %s to %s [Duration : %s]", start.String(), time.Now().String(), time.Since(start).String())
	return time.Since(start), nil
}

// ReloadConf stops the polling, reloads all configuration and restart the polling.
func ReloadConf() (time.Duration, error) {
	start := time.Now()
	log.Infof("RELOADCONF INIT: begin device Gather processes stop... at %s", start.String())
	End()

	log.Info("RELOADCONF: loading configuration Again...")

	log.Info("RELOADCONF: Starting all device processes again...")
	// Initialize Devices in Runtime map

	log.Infof("RELOADCONF END: Finished from %s to %s [Duration : %s]", start.String(), time.Now().String(), time.Since(start).String())

	return time.Since(start), nil
}