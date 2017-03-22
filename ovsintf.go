package goovs

import (
	"fmt"
	"reflect"

	"github.com/socketplane/libovsdb"
)

// OvsInterface is the structure represents an interface row
type OvsInterface struct {
	UUID    string            `json:"_uuid"`
	Name    string            `json:"name"`
	Options map[string]string `json:"options"`
	Type    string            `json:"type"`
}

// ReadFromDBRow is used to initialize the object from a row
func (intf *OvsInterface) ReadFromDBRow(row *libovsdb.Row) error {
	if intf.Options == nil {
		intf.Options = make(map[string]string)
	}
	for field, value := range row.Fields {
		switch field {
		case "name":
			intf.Name = value.(string)
		case "type":
			intf.Type = value.(string)
		case "options":
			for key, opt := range value.(libovsdb.OvsMap).GoMap {
				intf.Options[key.(string)] = opt.(string)
			}
               }
	}
	return nil
}

// AddInternalInterfaceOnPort ...
func (client *ovsClient) AddInternalInterfaceOnPort(portname string) error {
	// intf row to insert
	intf := make(map[string]interface{})
	intf["name"] = portname
	intf["type"] = `internal`

	return client.addInterfaceOnPort(portname, intf)
}

// AddVethInterfaceOnPort ...
func (client *ovsClient) AddVethInterfaceOnPort(portname string) error {
	// intf row to insert
	intf := make(map[string]interface{})
	intf["name"] = portname
	intf["type"] = `system`

	return client.addInterfaceOnPort(portname, intf)
}

// AddPeerInterfaceOnPort ...
func (client *ovsClient) AddPeerInterfaceOnPort(portname, peername string) error {
	// intf row to insert
	intf := make(map[string]interface{})
	intf["name"] = portname
	intf["type"] = `peer`
	intf["options"] = fmt.Sprintf("{peer=%s}", peername)

	return client.addInterfaceOnPort(portname, intf)
}

func (client *ovsClient) FindStatisticsOnInterface(intfname string) (map[string]float64, error) {
	condition := libovsdb.NewCondition("name", "==", intfname)
	selectOp := libovsdb.Operation{
		Op:      selectOperation,
		Table:   interfaceTableName,
		Where:   []interface{}{condition},
		Columns: []string{"statistics"},
	}
	operations := []libovsdb.Operation{selectOp}
	reply, _ := client.dbClient.Transact(defaultOvsDB, operations...)
	if len(reply) < len(operations) {
		return nil, fmt.Errorf("Get statistics from interface failed due to Number of Replies should be at least equal to number of Operations")
	}
	if reply[0].Error != "" {
		return nil, fmt.Errorf("Get statistics from interface failed due to Transaction Failed due to an error: %v", reply[0].Error)
	}
	if len(reply[0].Rows) == 0 {
		return nil, nil
	}

	infStatistics := make(map[string]float64)

	interfaceMap, _ := reply[0].Rows[0]["statistics"]
	interfaceMapAsSlice := interfaceMap.([]interface{})

	// Here we pick index 1 since index 0 is string "Set"
	intfStatisticsSet := interfaceMapAsSlice[1]
	intfStatisticsSetAsSlice := intfStatisticsSet.([]interface{})
	//statistics := reflect.ValueOf(intfStatisticsSet)
        for _, vintf := range intfStatisticsSetAsSlice{
                var key string
                var value float64
                for _, sintf := range vintf.([]interface{}) {
                        sv := reflect.ValueOf(sintf)
                        if sv.Kind() == reflect.String {
                                key = sintf.(string)
                        }
                        if sv.Kind() == reflect.Float64{
                                value = sintf.(float64)
                        }
                }
                infStatistics[key] = value
        }

	return infStatistics, nil
}

func (client *ovsClient) addInterfaceOnPort(portName string, intf map[string]interface{}) error {
	intfUpdateLock.Lock()
	defer intfUpdateLock.Unlock()
	namedInterfaceUUID := "gointerface"
	insertInterfaceOp := libovsdb.Operation{
		Op:       insertOperation,
		Table:    interfaceTableName,
		Row:      intf,
		UUIDName: namedInterfaceUUID,
	}
	// Inserting a Port row in Port table requires mutating the Bridge table
	mutateUUID := []libovsdb.UUID{libovsdb.UUID{GoUUID: namedInterfaceUUID}}
	mutateSet, _ := libovsdb.NewOvsSet(mutateUUID)
	mutation := libovsdb.NewMutation("interfaces", insertOperation, mutateSet)
	condition := libovsdb.NewCondition("name", "==", portName)

	// simple mutate operation
	mutateOp := libovsdb.Operation{
		Op:        mutateOperation,
		Table:     portTableName,
		Mutations: []interface{}{mutation},
		Where:     []interface{}{condition},
	}

	operations := []libovsdb.Operation{insertInterfaceOp, mutateOp}
	return client.transact(operations, "add interface")
}

func (client *ovsClient) RemoveInterfaceFromPort(portname, interfaceUUID string) error {
	intfUpdateLock.Lock()
	defer intfUpdateLock.Unlock()
	namedInterfaceUUID := "gointerface"
	interfaceDeleteCondition := libovsdb.NewCondition("_uuid", "==", []string{"uuid", interfaceUUID})
	interfaceDeleteOp := libovsdb.Operation{
		Op:    deleteOperation,
		Table: interfaceTableName,
		Where: []interface{}{interfaceDeleteCondition},
	}

	// Inserting a Port row in Port table requires mutating the Bridge table
	mutateUUID := []libovsdb.UUID{libovsdb.UUID{GoUUID: namedInterfaceUUID}}
	mutateSet, _ := libovsdb.NewOvsSet(mutateUUID)
	mutation := libovsdb.NewMutation("interfaces", deleteOperation, mutateSet)
	condition := libovsdb.NewCondition("name", "==", portname)

	// simple mutate operation
	mutateOp := libovsdb.Operation{
		Op:        mutateOperation,
		Table:     portTableName,
		Mutations: []interface{}{mutation},
		Where:     []interface{}{condition},
	}

	operations := []libovsdb.Operation{interfaceDeleteOp, mutateOp}
	return client.transact(operations, "remove interface")
}

func (client *ovsClient) interfaceUUIDExists(interfaceUUID string) (bool, error) {
	if interfaceUUID == "" {
		return false, fmt.Errorf("The interface uuid is not valid")
	}
	intfCacheUpdateLock.RLock()
	defer intfCacheUpdateLock.RUnlock()
	_, ok := client.interfaceCache[interfaceUUID]
	return ok, nil
}


func (client *ovsClient) findAllInterfaceUUIDOnPort(portname string) ([]string, error) {
	condition := libovsdb.NewCondition("name", "==", portname)
	selectOp := libovsdb.Operation{
		Op:      selectOperation,
		Table:   portTableName,
		Where:   []interface{}{condition},
		Columns: []string{"interfaces"},
	}
	operations := []libovsdb.Operation{selectOp}
	reply, _ := client.dbClient.Transact(defaultOvsDB, operations...)
	if len(reply) < len(operations) {
		return nil, fmt.Errorf("Get interface from port failed due to Number of Replies should be at least equal to number of Operations")
	}
	if reply[0].Error != "" {
		return nil, fmt.Errorf("Get interface from port failed due to Transaction Failed due to an error: %v", reply[0].Error)
	}
	if len(reply[0].Rows) == 0 {
		return nil, nil
	}
	var interfaceUUIDs []string
	interfaceMap, _ := reply[0].Rows[0]["interfaces"]
	interfaceMapAsSlice := interfaceMap.([]interface{})
	// Here we pick index 1 since index 0 is string "Set"
	interfaceSet := interfaceMapAsSlice[1]
	v := reflect.ValueOf(interfaceSet)
	if v.Kind() == reflect.String {
		interfaceUUIDs = append(interfaceUUIDs, interfaceSet.(string))
		return interfaceUUIDs, nil
	}

	interfaceSetAsSlice := interfaceSet.([]interface{})
	for _, singleSet := range interfaceSetAsSlice {
		for _, interfaceInfo := range singleSet.([]interface{}) {
			// Portinfo is a string with format: uuidXXXX (where XXX is the UUID of the port)
			interfaceID := interfaceInfo.(string)[4:]
			interfaceUUIDs = append(interfaceUUIDs, interfaceID)
		}
	}
	return interfaceUUIDs, nil
}
