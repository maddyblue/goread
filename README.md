# go read

a google reader clone built with go on app engine and angularjs

## setting up a local dev environment

1. install [Python 2.7](http://www.python.org/download/releases/2.7.5/) if you don't have it and make sure it is in your $PATH. Google App Engine doesn't yet work with 3.*.
1. install [Git](http://gitscm.com/) and [Mercurial](http://mercurial.selenic.com/wiki/Download) and make sure `git` and `hg` are in your $PATH.
1. install the [go app engine SDK](https://developers.google.com/appengine/downloads#Google_App_Engine_SDK_for_Go)
1. set your `GOPATH` (to something like `/home/user/mygo`), and make sure it's a directory that exists. (Note: Set this on your machine's environment, not in the go.bat file)
1. further commands that use `go`, `dev_appserver.py`, and `appcfg.py` all live in the `google_appengine` directory from the SDK. make sure it's in your $PATH.
1. download dependencies by running: `go get -d github.com/mjibson/goread/goapp`. this will download goread and all of its dependencies, and will stick them in your GOPATH.
1. `cd $GOPATH/src/github.com/mjibson/goread`
1. `git checkout master` (bug in `go get`)
1. in the `goapp` folder, copy `settings.go.dist` to `settings.go`
1. from the `goread` directory, start the app with `dev_appserver.py app.yaml`
1. view at [localhost:8080](http://localhost:8080), admin console at [localhost:8000](http://localhost:8000)
 
## developer notes

1. press `alt+c` to show the miniprofiler window
1. press `c` to clear all feeds and stories, remove all your subscriptions, and reset your unread date

## how to host your own on production app engine servers

1. set up a local dev environment as described above
1. create a new app engine application
1. copy `app.sample.yaml` to `app.yaml`
1. in `app.yaml`, change the first line to contain the name of the application you just created
1. from the `goread` directory, deploy with `appcfg.py update .`
