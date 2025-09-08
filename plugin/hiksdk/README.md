需要将lib下的linux或windows下的文件复制到main.go同级目录下,才能正常运行

配置文件

hik:
  loglevel: debug //根据实际情况修改
  client:
    - ip: "172.16.9.35"
      username: "admin"
      password: "123456"
      port: 8000
    - ip: "172.16.9.200"
      username: "admin"
      password: "123456"
      port: 8000