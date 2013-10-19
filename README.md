# go read

a google reader clone built with go on app engine and angularjs

## setting up a local dev environment

1. Install [Python 2.7](http://www.python.org/download/releases/2.7.5/) and make sure it is in your `PATH`. (Google App Engine doesn't yet work with Python 3.)
1. Install [Git](http://gitscm.com/) and [Mercurial](http://mercurial.selenic.com/wiki/Download) and make sure `git` and `hg` are in your `PATH`.
1. Install the [Go App Engine SDK](https://developers.google.com/appengine/downloads#Google_App_Engine_SDK_for_Go).
1. Set your `GOPATH` (to something like `/home/user/mygo`), and make sure it's a directory that exists. (Note: set this on your machine's environment, not in the go.bat file.)
1. Further commands that use `go`, `dev_appserver.py`, and `appcfg.py` all live in the `google_appengine` directory from the SDK. Make sure it's in your `PATH`.
1. Download dependencies by running: `go get -d github.com/mjibson/goread`. This will download goread and all of its dependencies, and will stick them in your `GOPATH`.
1. `cd $GOPATH/src/github.com/mjibson/goread`.
1. `git checkout master` (bug in `go get`).
1. Copy `app.sample.yaml` to `app.yaml`.
1. In the `goread` directory, copy `settings.go.dist` to `settings.go`.
1. From the `goread` directory, start the app with `dev_appserver.py app.yaml`. (On Windows, you may need to do this instead: `python C:\go_appengine\dev_appserver.py app.yaml`.)
1. View at [localhost:8080](http://localhost:8080), admin console at [localhost:8000](http://localhost:8000).
 
## developer notes

1. Press `alt+c` to show the miniprofiler window.
1. Press `c` to clear all feeds and stories, remove all your subscriptions, and reset your unread date.

## self host on production app engine servers

1. Set up a local dev environment as described above.
1. Create a [new app engine application](https://cloud.google.com/console?getstarted=https://appengine.google.com).
1. In `app.yaml`, change the first line to contain the name of the application you just created.
1. From the `goread` directory, deploy with `appcfg.py update .`. (On Windows, you may need to do this instead: `python C:\go_appengine\appcfg.py update .`)