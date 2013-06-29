# go read

a google reader clone built with go on app engine and angularjs

## setting up a local dev environment

1. install [Python 2.7.3](http://www.python.org/download/releases/2.7.3/#id5) if you don't have it and make sure it is in your $ATH. Google App Engine doesn't yet work with 3.*.
1. install [Mercurial](http://mercurial.selenic.com/wiki/Download) and make sure hg.exe is in your path.
1. checkout the code
1. install the [go app engine SDK](https://developers.google.com/appengine/downloads#Google_App_Engine_SDK_for_Go)
1. set your GOPATH (to something like `/home/user/mygo`), and make sure it's a directory that exists
1. further commands that use `go`, `dev_appserver.py`, and `appcfg.py` all live in the `google_appengine` directory from the SDK. make sure it's in your `$PATH`.
1. download dependencies by running: `go get -d github.com/mjibson/goread/goapp`. although you've already checked out the code for development use, this will automatically download all of goread's dependencies, and will stick them all in your GOPATH.
1. in the `goapp` folder, copy `settings.go.dist` to `settings.go`
1. from the `goread` directory, start the app with `dev_appserver.py .`

## how to host your own on production app engine servers

1. create a new app engine application
1. in `app.yaml`, change the first line to contain the name of the application you just created

[optional steps if you want google reader import support]

1. sign up for some API keys at the [google apis console](https://code.google.com/apis/console/)
1. fill in values for localhost and whatever your hostname is in the `init()` function of `settings.go`
1. get a google analytics key and put it into the appropriate field in `settings.go`

[and finally, deploy]

1. from the `goread` directory, deploy with `appcfg.py update .`
