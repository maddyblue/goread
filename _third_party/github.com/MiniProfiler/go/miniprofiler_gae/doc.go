/*
 * Copyright (c) 2013 Matt Jibson <matt.jibson@gmail.com>
 *
 * Permission to use, copy, modify, and distribute this software for any
 * purpose with or without fee is hereby granted, provided that the above
 * copyright notice and this permission notice appear in all copies.
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

/*
Package miniprofiler_gae is a simple but effective mini-profiler for app engine.

miniprofiler_gae hooks into the appstats package, and all app engine RPCs are automatically profiled.
An appstats link is listed in each Profile.

To use this package, change your HTTP handler functions to use this signature:

    func(mpg.Context, http.ResponseWriter, *http.Request)

Register them in the usual way, wrapping them with NewHandler.

Send output of c.Includes() to your HTML (it is empty if Enable returns
false).

By default, miniprofiler_gae is enabled on dev for all and on prod for admins.
Override miniprofiler.Enable to change.

Step

Unlike base miniprofiler, the Step function returns a profiled context:

    c.Step("something", func(c mpg.Context) {
        // c is valid appengine.Context and miniprofiler.Timer:
        // datastore.Get(c, key, entity)
        // c.Step("another", func(c mpg.Context) { ... })
    })

See the miniprofiler package docs about further usage: http://godoc.org/github.com/MiniProfiler/go/miniprofiler.
*/
package miniprofiler_gae
