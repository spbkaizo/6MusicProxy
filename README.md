# Overview 
This is a small utility which basically creates a web server, which you can connect
to on http://hostname:8888/index.m3u8 which keeps a local copy of the current HLS
stream from 6 Music.

This basically reduces the load on the BBC servers, if you have multirooms connecting
to the upstream stream.  It has a circular buffer in memory where it keeps the current
data, and the index.m3u8 is automatically generated.  It does however write out a log 
file and also a small state file (used to stop streams rewinding if you restart the 
service).

I've had this running about a year, its pretty stable and handles router reboots well,
if the rooter reboots in a short time then you should not hear any breaks in the streams
which used to drive me mad!

# Log file format 

```
2020/06/21 08:50:24 main.go:259: TRACK:  bbc_6music-audio=320000-248863415.ts downloaded in 225ms, data total: 118.3 GB, uptime: 785h14m39s
2020/06/21 08:50:29 main.go:319: REMOTE: Client from 172.16.0.118:40730 served index.m3u8 in 41.555_s [Lavf/58.20.100]
2020/06/21 08:50:29 main.go:319: REMOTE: Client from 172.16.0.1:55908 served index.m3u8 in 22.489_s [Lavf/58.29.100]
2020/06/21 08:50:29 main.go:319: REMOTE: Client from 172.16.0.1:28513 served bbc_6music-audio=320000-248863415.ts in 408.702_s [Lavf/58.29.100]
2020/06/21 08:50:29 main.go:319: REMOTE: Client from 172.16.0.118:40876 served bbc_6music-audio=320000-248863415.ts in 60.073857ms [Lavf/58.20.100]
2020/06/21 08:50:30 main.go:259: TRACK:  bbc_6music-audio=320000-248863416.ts downloaded in 178ms, data total: 118.3 GB, uptime: 785h14m45s
2020/06/21 08:50:35 main.go:319: REMOTE: Client from 172.16.0.118:40730 served index.m3u8 in 23.466_s [Lavf/58.20.100]
2020/06/21 08:50:35 main.go:319: REMOTE: Client from 172.16.0.1:55908 served index.m3u8 in 23.466_s [Lavf/58.29.100]
2020/06/21 08:50:35 main.go:319: REMOTE: Client from 172.16.0.1:28513 served bbc_6music-audio=320000-248863416.ts in 574.921_s [Lavf/58.29.100]
2020/06/21 08:50:35 main.go:319: REMOTE: Client from 172.16.0.118:40876 served bbc_6music-audio=320000-248863416.ts in 69.087795ms [Lavf/58.20.100]
```

# Memory Usage 

```
zsh/2 1024  (git)-[master]-% ps axu | grep 6music
nobody     67279   0.0  0.3  127628   25424  -  Ss   19May20   318:54.16 /usr/local/bin/6music
```
