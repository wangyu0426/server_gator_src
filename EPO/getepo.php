<?php
system('rm -rf  /home/work/server-go-src/EPO/36H.EPO; curl -o /home/work/server-go-src/EPO/36H.EPO  -F username=getepo -F  password=n8vZB9belqO4ydnx  http://service.gatorcn.com/tracker/web/download36h.php?action=down36h');
if(filesize('/home/work/server-go-src/EPO/36H.EPO') !== 27648){
	echo "get epo err: " . "\n";
}else{
	echo "get epo -- ok\n";
	system('redis-cli -p 9015 reload epo');
}
?>
