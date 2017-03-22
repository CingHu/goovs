package goovs

import (
	"fmt"
	"net"
	"reflect"
	"strconv"
	"sync"

	"github.com/socketplane/libovsdb"
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
        AddInternalInterfaceOnPort(portname string) error
        FindStatisticsOnInterface(intfname string) (map[string]float64, error)
	Disconnect()
}

type ovsClient struct {
	dbClient       *libovsdb.OvsdbClient
	bridgeCache    map[string]*OvsBridge
	portCache      map[string]*OvsPort
	interfaceCache map[string]*OvsInterface
}

var client *ovsClient
var cache map[string]map[string]libovsdb.Row
var ovsclient *ovsClient

var bridgeUpdateLock sync.RWMutex
var portUpdateLock sync.RWMutex
var intfUpdateLock sync.RWMutex

var bridgeCacheUpdateLock sync.RWMutex
var portCacheUpdateLock sync.RWMutex
var intfCacheUpdateLock sync.RWMutex
var populateCacheLock sync.RWMutex

// GetOVSClient is used for
func GetOVSClient(contype, endpoint string) (OvsClient, error) {
	if client != nil {
		return client, nil
	}
	var dbclient *libovsdb.OvsdbClient
	var err error
	switch contype {
	case "tcp":
		if endpoint == "" {
			dbclient, err = libovsdb.Connect(defaultTCPHost, defaultTCPPort)
		} else {
			var host, port string
			host, port, err = net.SplitHostPort(endpoint)
			if err != nil {
				return nil, err
			}
			var portInt int
			if portInt, err = strconv.Atoi(port); err != nil {
				return nil, err
			}
			dbclient, err = libovsdb.Connect(host, portInt)
		}
	case "unix":
		if endpoint == "" {
			endpoint = defaultUnixEndpoint
		}
		dbclient, err = libovsdb.ConnectWithUnixSocket(endpoint)
	default:
		return nil, fmt.Errorf("GetOVSClient: Unsupported connection type %q.", contype)
	}
	if err != nil {
		return nil, err
	}
	var notfr notifier
	dbclient.Register(notfr)

	cache = make(map[string]map[string]libovsdb.Row)

	client = &ovsClient{dbClient: dbclient}
	if client.bridgeCache == nil {
		client.bridgeCache = make(map[string]*OvsBridge)
	}
	if client.portCache == nil {
		client.portCache = make(map[string]*OvsPort)
	}
	if client.interfaceCache == nil {
		client.interfaceCache = make(map[string]*OvsInterface)
	}

	var initial *libovsdb.TableUpdates
	initial, err = dbclient.MonitorAll(defaultOvsDB, "")
	if err != nil {
		return nil, err
	}
	populateCache(*initial)
	return client, nil
}

func (client *ovsClient) Disconnect() {
	client.dbClient.Disconnect()
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
	//	log.Println(action, "successful: ", reply[0].UUID.GoUUID)
	//}

	return nil
}

type notifier struct {
}

func (n notifier) Update(context interface{}, tableUpdates libovsdb.TableUpdates) {
	populateCache(tableUpdates)
}
func (n notifier) Locked([]interface{}) {
}
func (n notifier) Stolen([]interface{}) {
}
func (n notifier) Echo([]interface{}) {
}
func (n notifier) Disconnect([]interface{}) {
}
func (n notifier) Disconnected(*libovsdb.OvsdbClient) {
}

func (client *ovsClient) updateOvsObjCacheByRow(objtype, uuid string, row *libovsdb.Row) (err error) {
	switch objtype {
	case bridgeTableName:
		brObj := &OvsBridge{UUID: uuid}
		if err = brObj.ReadFromDBRow(row); err != nil {
			return
		}
		bridgeCacheUpdateLock.Lock()
		client.bridgeCache[uuid] = brObj
		bridgeCacheUpdateLock.Unlock()
		//data, _ := json.MarshalIndent(brObj, "", "    ")
		//fmt.Println(string(data))
	case portTableName:
		portObj := &OvsPort{UUID: uuid}
		if err = portObj.ReadFromDBRow(row); err != nil {
			return
		}
		portCacheUpdateLock.Lock()
		client.portCache[uuid] = portObj
		portCacheUpdateLock.Unlock()
		//data, _ := json.MarshalIndent(portObj, "", "    ")
		//fmt.Println(string(data))
	case interfaceTableName:
		intfObj := &OvsInterface{UUID: uuid}
		if err = intfObj.ReadFromDBRow(row); err != nil {
			return
		}
		intfCacheUpdateLock.Lock()
		client.interfaceCache[uuid] = intfObj
		intfCacheUpdateLock.Unlock()
		//data, _ := json.MarshalIndent(intfObj, "", "    ")
		//fmt.Println(string(data))
	}
	return
}

func (client *ovsClient) removeOvsObjCacheByRow(objtype, uuid string) error {
	switch objtype {
	case bridgeTableName:
		if _, ok := client.bridgeCache[uuid]; ok {
			bridgeCacheUpdateLock.Lock()
			delete(client.bridgeCache, uuid)
			bridgeCacheUpdateLock.Unlock()
		}
	case portTableName:
		if _, ok := client.portCache[uuid]; ok {
			portCacheUpdateLock.Lock()
			delete(client.portCache, uuid)
			portCacheUpdateLock.Unlock()
		}
	case interfaceTableName:
		if _, ok := client.interfaceCache[uuid]; ok {
			intfCacheUpdateLock.Lock()
			delete(client.interfaceCache, uuid)
			intfCacheUpdateLock.Unlock()
		}
	}
	return nil
}

func populateCache(updates libovsdb.TableUpdates) (err error) {
	populateCacheLock.Lock()
	defer populateCacheLock.Unlock()
	for table, tableUpdate := range updates.Updates {
		if _, ok := cache[table]; !ok {
			cache[table] = make(map[string]libovsdb.Row)

		}
		for uuid, row := range tableUpdate.Rows {
			empty := libovsdb.Row{}
			if !reflect.DeepEqual(row.New, empty) {
				// fmt.Println(table + " with uuid " + uuid + "is updated")
				cache[table][uuid] = row.New
				if err = client.updateOvsObjCacheByRow(table, uuid, &row.New); err != nil {
					return
				}
			} else {
				delete(cache[table], uuid)
				// fmt.Println(table + " with uuid " + uuid + "is removed")
				if err = client.removeOvsObjCacheByRow(table, uuid); err != nil {
					return
				}
			}
		}
	}
	return
}

func getRootUUID() string {
	for uuid := range cache[defaultOvsDB] {
		return uuid
	}
	return ""
}
