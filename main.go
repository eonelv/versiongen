// versiongen project main.go
package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	. "taskmd5"
	"time"

	"ngcod.com/utils"

	. "ngcod.com/core"
)

var RootPath string
var isServer bool

func main() {
	start()
}

func RemoveJsonKey() {
	oldJson, err := utils.ReadJson("C:/eonegame/UnrealEngine/reslist.json")
	if err != nil {
		return
	}
	mapData := oldJson.MustMap()
	for k, _ := range mapData {
		if strings.HasSuffix(k, ".pch") {
			oldJson.Del(k)
			LogError("Delete", k)
		}
	}
	Bytes, err := oldJson.MarshalJSON()
	if err != nil {
		LogError("Read Json Data Error!", err)
	}
	err = utils.WriteFile(Bytes, fmt.Sprintf("%s/reslist.json", "C:/eonegame/UnrealEngine"))
	if err != nil {
		LogError("Write reslist.json Error!", err)
	}
}

func start() {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		return
	}

	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	RootPath = dir

	//UE4强制修改为Install版本
	SourceBuildFileName := RootPath + "/Engine/Build/SourceDistribution.txt"
	InstalledBuildFileName := RootPath + "/Engine/Build/InstalledBuild.txt"
	InstalledBuildFileNameSrc := RootPath + "/Engine/Build/BuildType/InstalledBuild.txt"
	os.Remove(SourceBuildFileName)
	utils.CopyFile(InstalledBuildFileNameSrc, InstalledBuildFileName)

	isServer = false
	if len(os.Args) > 2 {
		isServer, _ = strconv.ParseBool(os.Args[2])
		fmt.Println("isServer=", isServer)
	}

	oldJson, err := utils.ReadJson(dir + "/restimelist.json")
	oldJsonMD5, err := utils.ReadJson(dir + "/reslist_srv.json")

	beginTime := time.Now()
	filemd5 := &PakMD5{}
	filemd5.LocalFileTime = oldJson.MustMap()
	filemd5.LocalFileMD5 = oldJsonMD5.MustMap()
	filemd5.CalcMD5(dir)

	timePassed := time.Now().Unix() - beginTime.Unix()

	timeString := fmt.Sprintf("耗时:%dh:%dm:%ds(%ds).", timePassed/60/60, timePassed%3600/60, timePassed%3600%60, timePassed)
	fmt.Println(timeString)

	if isServer {
		startFileServer()
	}
	for {
		select {
		case <-time.After(10 * time.Second):
			return
		}
	}
}

func startFileServer() {
	//p, _ := filepath.Abs("F:/UnrealEngine4.24.3")
	p, _ := filepath.Abs(RootPath)
	http.Handle("/", http.FileServer(http.Dir(p)))

	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println(err)
	}
}
