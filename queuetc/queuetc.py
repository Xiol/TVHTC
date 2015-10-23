#!/usr/bin/env python
import json
import os
import sys
import requests
from syslog import syslog

if len(sys.argv) != 6:
    print "Missing arguments."
    print "Usage: tvheadend_postprocess.py {path} {channel} {title} {status} {description}"
    print 'Your postprocessor command in tvheadend should be: /path/to/queuetc.py "%f" "%c" "%t" "%e" "%d"'
    sys.exit(1)

payload = dict(path=sys.argv[1], fname=os.path.basename(sys.argv[1]), channel=sys.argv[2],
                 title=sys.argv[3], status=sys.argv[4], description=sys.argv[5])

hdr = { "Content-type": "application/json" }
r = requests.post("http://127.0.0.1:8998/job", data=json.dumps(payload), headers=hdr)

try:
    if r.status_code != 200 or r.json()['status'] != "ok":
        syslog("Error queueing job: {}".format(r.body))
except Exception as e:
    syslog("Error queueing job: {}".format(e.message))


