package main

import (
	"log"
	"os"

	"goovs"
)

func main() {
        if len(os.Args) != 2 {
                log.Println("Error: please input interface name")
                return
        }

        intfName := os.Args[1]

	client, err := goovs.GetOVSClient("unix", "")
	if err != nil {
		log.Println(err.Error())
		return
	}
	portStatistics, err := client.FindStatisticsOnInterface(intfName)
	if err != nil {
		log.Println(err.Error())
	}

        log.Println(portStatistics)


}
