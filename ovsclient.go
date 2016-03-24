package goovs

import (
	"fmt"
	"net"
	"reflect"
	"strconv"
	"sync"

	"github.com/kopwei/libovsdb"
)

const (
	defaultTCPHost      = "127.0.0.1"
	defaultTCPPort      = 6640
	defaultUnixEndpoint = "/var/run/openvswitch/db.sock"
)

const (
	defaultOvsDB = "Open_vSwitch"
)

const (
	ovsTableName        = "Open_vSwitch"
	bridgeTableName     = "Bridge"
	portTableName       = "Port"
	interfaceTableName  = "Interface"
	controllerTableName = "Controller"
	//flowTableName = ""
)

const (
	deleteOperation = "delete"
	insertOperation = "insert"
	mutateOperation = "mutate"
	selectOperation = "select"
	updateOperation = "update"
)

// OvsObject is the main interface represent an ovs object
type OvsObject interface {
	ReadFromDBRow(row *libovsdb.Row) error
}

// OvsClient is the interface towards outside user
type OvsClient interface {
	BridgeExists(brname string) (bool, error)
	CreateBridge(brname string) error
	DeleteBridge(brname string) error
	UpdateBridgeController(brname, controller string) error
	CreateInternalPort(brname, portname string, vlantag int) error
	CreateVethPort(brname, portname string, vlantag int) error
	CreatePatchPort(brname, portname, peername string) error
	DeletePort(brname, porname string) error
	UpdatePortTagByName(brname, portname string, vlantag int) error
	FindAllPortsOnBridge(brname string) ([]string, error)
	PortExistsOnBridge(portname, brname string) (bool, error)
	RemoveInterfaceFromPort(portname, interfaceUUID string) error
}

type ovsClient struct {
	dbClient *libovsdb.OvsdbClient
}

var client *ovsClient
var update chan *libovsdb.TableUpdates
var cache map[string]map[string]libovsdb.Row

var bridgeUpdateLock sync.RWMutex
var portUpdateLock sync.RWMutex
var intfUpdateLock sync.RWMutex

// GetOVSClient is used for
func GetOVSClient(contype, endpoint string) (OvsClient, error) {
	if client != nil {
		return client, nil
	}
	var dbclient *libovsdb.OvsdbClient
	var err error
	if contype == "tcp" {
		if endpoint == "" {
			dbclient, err = libovsdb.Connect(defaultTCPHost, defaultTCPPort)
		} else {
			host, port, err := net.SplitHostPort(endpoint)
			if err != nil {
				return nil, err
			}
			portInt, _ := strconv.Atoi(port)
			dbclient, err = libovsdb.Connect(host, portInt)
		}
	} else if contype == "unix" {
		if endpoint == "" {
			endpoint = defaultUnixEndpoint
		}
		dbclient, err = libovsdb.ConnectUnix(endpoint)
	}
	if err != nil {
		return nil, err
	}
	//var notifier Notifier
	//dbclient.Register(notifier)

	//update = make(chan *libovsdb.TableUpdates)
	cache = make(map[string]map[string]libovsdb.Row)

	initial, _ := dbclient.MonitorAll(defaultOvsDB, "")
	populateCache(*initial)

	client = &ovsClient{dbClient: dbclient}
	return client, nil
}

func (client *ovsClient) transact(operations []libovsdb.Operation, action string) error {
	reply, _ := client.dbClient.Transact(defaultOvsDB, operations...)

	if len(reply) < len(operations) {
		return fmt.Errorf("%s failed due to Number of Replies should be at least equal to number of Operations", action)
	}
	//ok := true
	for i, o := range reply {
		if o.Error != "" {
			//ok = false
			if i < len(operations) {
				return fmt.Errorf("%s transaction Failed due to an error : %s details: %s in %+v", action, o.Error, o.Details, operations[i])
			}
			return fmt.Errorf("%s transaction Failed due to an error :%s", action, o.Error)
		}
	}
	//if ok {
	//	log.Println(action, "successful: ", reply[0].UUID.GoUuid)
	//}

	return nil
}

/*
type Notifier struct {
}

func (n Notifier) Update(context interface{}, tableUpdates libovsdb.TableUpdates) {
	populateCache(tableUpdates)
	update <- &tableUpdates
}
func (n Notifier) Locked([]interface{}) {
}
func (n Notifier) Stolen([]interface{}) {
}
func (n Notifier) Echo([]interface{}) {
}
*/

func updateOvsObjCacheByRow(objtype, uuid string, row *libovsdb.Row) error {
	if bridgeCache == nil {
		bridgeCache = make(map[string]*OvsBridge)
	}
	if portCache == nil {
		portCache = make(map[string]*OvsPort)
	}
	if interfaceCache == nil {
		interfaceCache = make(map[string]*OvsInterface)
	}
	switch objtype {
	case "bridge":
		brObj := &OvsBridge{UUID: uuid}
		brObj.ReadFromDBRow(row)
		bridgeCache[uuid] = brObj
	case "port":
		portObj := &OvsPort{UUID: uuid}
		portObj.ReadFromDBRow(row)
		portCache[uuid] = portObj
	case "interface":
		intfObj := &OvsInterface{UUID: uuid}
		intfObj.ReadFromDBRow(row)
		interfaceCache[uuid] = intfObj
	}
	return nil
}

func populateCache(updates libovsdb.TableUpdates) {
	for table, tableUpdate := range updates.Updates {
		if _, ok := cache[table]; !ok {
			cache[table] = make(map[string]libovsdb.Row)

		}
		for uuid, row := range tableUpdate.Rows {
			empty := libovsdb.Row{}
			if !reflect.DeepEqual(row.New, empty) {
				// fmt.Printf("table name:%s\n. row info %+v\n\n", table, row.New)
				cache[table][uuid] = row.New
				updateOvsObjCacheByRow(table, uuid, &row.New)
			} else {
				delete(cache[table], uuid)
			}
		}
	}
}

func getRootUUID() string {
	for uuid := range cache[defaultOvsDB] {
		return uuid
	}
	return ""
}
