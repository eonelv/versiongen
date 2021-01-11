// taskmd5
package taskmd5

import (
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"strings"
	"time"

	"ngcod.com/utils"

	. "ngcod.com/core"

	simplejson "github.com/bitly/go-simplejson"
)

var cppSize int64
var pdbSize int64

type ResFileInfo struct {
	name       string
	md5        string
	size       int64
	modifytime int64
}

type SFileInfo struct {
	name       string
	size       int64
	modifytime int64
}

type ResFilePair struct {
	Key      string
	FileInfo *ResFileInfo
}

type PakMD5 struct {
	numCPU        int
	isPatch       bool
	path          string
	MD5           map[string]*ResFileInfo
	chanMD5       chan *ResFilePair
	chanFileName  chan *SFileInfo
	isInit        bool
	LocalFileTime map[string]interface{}
	LocalFileMD5  map[string]interface{}
}

type MD5Task struct {
	BaseMultiThreadTask
	channel       chan *SFileInfo
	chanMD5       chan *ResFilePair
	path          string
	version       int64
	LocalFileTime map[string]interface{}
}

func (this *MD5Task) CreateChan() {
	this.channel = make(chan *SFileInfo)
}

func (this *MD5Task) CloseChan() {
	close(this.channel)
}

func (this *MD5Task) WriteToChannel(SrcFileDir string) {
	rd, err := ioutil.ReadDir(SrcFileDir)
	if err != nil {
		LogError(err)
		return
	}
	for _, fi := range rd {
		if fi.IsDir() {
			isSVN := fi.Name() == ".svn"
			isVS := fi.Name() == ".vs"

			isCacheBuild := (fi.Name() == "Build" && strings.HasSuffix(SrcFileDir, "Intermediate"))
			isCacheBuild = false

			isDerivedDataCache := fi.Name() == "DerivedDataCache" && strings.HasSuffix(SrcFileDir, "Engine")

			isNeedGoTo := !isSVN && !isVS && !isCacheBuild && !isDerivedDataCache
			if isNeedGoTo {
				this.WriteToChannel(SrcFileDir + "/" + fi.Name())
			} else {
				LogInfo("forget folder:", SrcFileDir+"/"+fi.Name())
			}
		} else {
			Name := SrcFileDir + "/" + fi.Name()
			filetime := fi.ModTime().UnixNano()
			RelName := string(Name[strings.Count(this.path, ""):])
			filetimeJson := utils.GetInt(this.LocalFileTime, RelName)

			//根据时间对比文件是否改变, 如果时间不变,不计算MD5
			if filetime == filetimeJson {
				this.channel <- &SFileInfo{"", 0, 0}
				continue
			}

			isSourceFile := strings.HasSuffix(RelName, ".cpp")
			isSourceFile = false

			isPdbFile := strings.HasSuffix(RelName, ".pdb")
			isReslistFile := fi.Name() == "restimelist.json" || fi.Name() == "reslist_srv.json"

			if isSourceFile {
				cppSize += fi.Size()
			}
			if isPdbFile {
				pdbSize += fi.Size()
			}
			if isSourceFile || isPdbFile || isReslistFile {
				this.channel <- &SFileInfo{"", 0, 0}
				continue
			}
			LogDebug("Next Calc:", RelName)
			this.channel <- &SFileInfo{Name, fi.Size(), filetime}
		}
	}
}
func (this *MD5Task) ProcessTask(DestFileDir string) {
	for {
		select {
		case s := <-this.channel:
			if s.name == "" {
				break
			}
			md5 := utils.CalcFileMD5(s.name)
			RelName := string(s.name[strings.Count(this.path, ""):])

			fileInfo := &ResFileInfo{}
			fileInfo.name = RelName
			fileInfo.md5 = md5
			fileInfo.size = s.size
			fileInfo.modifytime = s.modifytime

			this.chanMD5 <- &ResFilePair{RelName, fileInfo}
		case <-time.After(2 * time.Second):
			return
		}
	}
}

func (this *PakMD5) CalcMD5(path string) {
	LogInfo("**********", "Begin calc New MD5 for pak. Server Root Folder Path:", path, "**********")

	this.path = path
	if !this.isInit {
		this.MD5 = make(map[string]*ResFileInfo)
	}
	completeChan := make(chan bool)
	defer close(completeChan)

	this.chanMD5 = make(chan *ResFilePair, runtime.NumCPU())
	defer close(this.chanMD5)
	go this.writeNewMD5(completeChan)

	var multiThreadTask *MD5Task = &MD5Task{}
	multiThreadTask.LocalFileTime = this.LocalFileTime
	multiThreadTask.chanMD5 = this.chanMD5
	multiThreadTask.path = path
	ExecTask(multiThreadTask, path, "")

	completeChan <- true
	LogInfo("**********Calc MD5 Complete**********")
	LogInfo("CPP size:", cppSize/1024/1024, "PDB size:", pdbSize/1024/1024)

	this.writeReslist()
	this.writeTimeJson()
}

//最重要的问题：
//1. 内网和外网包pakIndex怎么处理
//2. 内网和外网包的version怎么处理
func (this *PakMD5) writeReslist() {
	var ReslistData *simplejson.Json = simplejson.New()
	for K, V := range this.LocalFileMD5 {
		ReslistData.Set(K, V)
	}
	for Key := range this.MD5 {
		d := this.MD5[Key]
		itemC := simplejson.New()
		itemC.Set("size", d.size)
		itemC.Set("md5", d.md5)
		ReslistData.Set(Key, itemC)
	}
	Bytes, err := ReslistData.MarshalJSON()
	if err != nil {
		LogError("Read Json Data Error!", err)
	}
	err = writeFile(Bytes, fmt.Sprintf("%s/reslist_srv.json", this.path))
	if err != nil {
		LogError("Write reslist.json Error!", err)
	}
}

func (this *PakMD5) writeTimeJson() {
	var ReslistData *simplejson.Json = simplejson.New()
	for K, V := range this.LocalFileTime {
		ReslistData.Set(K, V)
	}
	for Key := range this.MD5 {
		d := this.MD5[Key]
		ReslistData.Set(Key, d.modifytime)
	}
	Bytes, err := ReslistData.MarshalJSON()
	if err != nil {
		LogError("Read Json Data Error!", err)
	}
	err = writeFile(Bytes, fmt.Sprintf("%s/restimelist.json", this.path))
	if err != nil {
		LogError("Write reslist.json Error!", err)
	}
}

func (this *PakMD5) writeNewMD5(completeChan chan bool) {
	for {
		select {
		case FileMD5 := <-this.chanMD5:
			this.MD5[FileMD5.Key] = FileMD5.FileInfo
		case <-completeChan:
			return
		}
	}
}

func writeFile(data []byte, filePath string) error {
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.ModePerm)
	defer f.Close()

	if err != nil {
		fmt.Println(err.Error())
		return err
	}
	_, err = f.Write(data)
	if err != nil {
		fmt.Println(err.Error())
		return err
	}
	return nil
}
