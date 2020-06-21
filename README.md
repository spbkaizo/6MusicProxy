
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
