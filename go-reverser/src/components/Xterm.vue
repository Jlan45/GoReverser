<script setup lang="js">
import { Terminal } from 'xterm'
import { FitAddon } from 'xterm-addon-fit'
import 'xterm/css/xterm.css'
import 'xterm/lib/xterm.js'
import axios from 'axios'
</script>
<script lang="js">
import {ref} from "vue";
import {ElNotification} from "element-plus";

export default {
  name: 'Xterm',
  data() {
    return {
      term: null,
      socket: null,
      username: ref(''),
      keystr: ref(''),
    }
  },
  methods:{
    onWebSocketClose(){
      ElNotification({
        title: '连接关闭',
        message: 'WebSocket连接已关闭',
        type: 'fail',
      })
    },
    createListener(){
      axios.get("http://127.0.0.1:8080/createNewListener").then((res)=>{
        ElNotification({
          title: '创建成功',
          message: '监听创建成功，请连接'+res.data.IP+':'+res.data.Port,
          type: 'success',
        })
        this.keystr=res.data.Key
      })
    },
    sendData(data){
      this.socket.send(JSON.stringify({
        type: 0,
        data: data,
      }))
    },
    handleWebSocketData(data){
      var dataObj= JSON.parse(data)
      console.log(dataObj)
      console.log(this.term)

      switch (dataObj.type) {
        case 0:
          var datasp=dataObj.data.split("\n");
          console.log(datasp)
            if(datasp.length==1){
              this.term.write(datasp[0])
            }
            else {
              for (const dataspKey in datasp) {
                this.term.writeln(datasp[dataspKey])
              }
            }

          break
        case 2:
          ElNotification({
            title: 'Server Message',
            message: dataObj.data,
            type: 'info',
          })
          break
        case 'disconnect':
          this.socket.close()
          break
      }
    },
    initTerm(){
      this.term = new Terminal({
        theme: {
          background: '#000000',
          foreground: '#ffffff',
        },
        cursorBlink: true,
        cursorStyle: 'underline',
        fontSize: 14,
        scrollback: 1000,
        tabStopWidth: 8,
        // theme: AdventureTime,
      })
      document.getElementById('xterm').innerHTML=""
      this.term.open(document.getElementById('xterm'))
      this.term.loadAddon(new FitAddon())
      this.term.onData((data) => {
        console.log(data)
        if (data=="\r"){
          data="\n"
        }
        this.sendData(data)
      })
      this.term.focus()
      this.term.write("hello")
    },
    onWebSocketOpen(){
      ElNotification({
        title: '连接成功',
        message: 'WebSocket连接已建立',
        type: 'success',
      })
      this.initTerm()
    },
    initWebSocket(){
      if(this.socket!=null){
        this.socket.close()
      }
      this.socket = new WebSocket('ws://127.0.0.1:8080/ws?name=' + this.username + '&key=' + this.keystr)
      if(this.socket==null){
        ElNotification({
          title: '连接失败',
          message: 'WebSocket连接失败',
          type: 'error',
        })
        return
      }
      this.socket.onopen=this.onWebSocketOpen
      this.socket.onmessage= (event) => {
        this.handleWebSocketData(event.data)
      }
      this.socket.onclose=this.onWebSocketClose
    },
  }
}
</script>
<template>
  <el-input
    v-model="username"
    clearable
    placeholder="请输入用户名"
    prefix-icon="Connection"
    style="width: 240px"
    />
  <el-input
    v-model="keystr"
    clearable
    placeholder="请输入Key"
    prefix-icon="Connection"
    style="width: 240px"
    />
  <el-button prefix-icon="Connection" @click="initWebSocket">加入shell</el-button>
  <el-button @click="createListener">新建监听</el-button>
  <div id="xterm"></div>
</template>

<style scoped>

</style>