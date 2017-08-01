<?php
system('rm -rf  36H.EPO; curl -o 36H.EPO  -F username=getepo -F  password=n8vZB9belqO4ydnx  http://service.gatorcn.com/tracker/web/download36h.php?action=down36h');
if(filesize('36H.EPO') !== 27648){
	echo "get epo err: " . file_get_contents('36H.EPO') . "\n";
}else{
	echo "get epo -- ok\n";
	system('redis-cli -p 9015 reload epo');
}
?>