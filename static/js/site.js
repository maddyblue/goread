$('.dropdown-toggle').dropdown();

if ($('#messages').attr('data-show')) {
	$('#messages').modal();
}

function countProperties(obj) {
	var count = 0;
	for(var prop in obj) {
		if(obj.hasOwnProperty(prop))
			++count;
	}
	return count;
}

var goReadAppModule = angular.module('goReadApp', ['ui.sortable']);
goReadAppModule.controller('GoreadCtrl', function($scope, $http, $timeout, $window) {
	$scope.loading = 0;
	$scope.contents = {};
	$scope.opts = {
		folderClose: {},
		nav: true,
		expanded: false,
		mode: 'unread',
		sort: 'newest',
		hideEmpty: false
	};

	$scope.sortableOptions = {
		stop: function() {
			$scope.uploadOpml();
		}
	};

	$scope.importOpml = function() {
		$scope.shown = 'feeds';
		$scope.loading++;
		$('#import-opml-form').ajaxForm(function() {
			$('#import-opml-form')[0].reset();
			$scope.loaded();
			$scope.showMessage("OPML import is happening. It can take a minute. Don't reorganize your feeds until it's completed importing. Refresh to see its progress.");
		});
	};

	$scope.loaded = function() {
		$scope.loading--;
	};

	$scope.http = function(method, url, data) {
		return $http({
			method: method,
			url: url,
			data: $.param(data || ''),
			headers: {'Content-Type': 'application/x-www-form-urlencoded'}
		});
	};

	$scope.addSubscription = function() {
		if (!$scope.addFeedUrl) {
			return false;
		}
		$scope.loading++;
		var f = $('#add-subscription-form');
		$scope.http('POST', f.attr('data-url'), {
			url: $scope.addFeedUrl
		}).then(function() {
			$scope.addFeedUrl = '';
			// I think this is needed due to the datastore's eventual consistency.
			// Without the delay we only get the feed data with no story data.
			$timeout(function() {
				$scope.refresh($scope.loaded);
			}, 250);
		}, function(data) {
			if (data.data) {
				alert(data.data);
			}
			$scope.loading--;
		});
	};

	$scope.procStory = function(xmlurl, story, read) {
		story.read = read;
		story.feed = $scope.xmlurls[xmlurl];
		story.guid = xmlurl + '|' + story.Id;
		if (!story.Title) {
			story.Title = '(title unknown)';
		}
		var today = new Date().toDateString();
		var d = new Date(story.Date * 1000);
		story.dispdate = moment(d).format(d.toDateString() == today ? "h:mm a" : "MMM D, YYYY");
	};

	$scope.refresh = function(cb) {
		$scope.loading++;
		$scope.shown = 'feeds';
		delete $scope.currentStory;
		$http.get($('#refresh').attr('data-url-feeds'))
			.success(function(data) {
				$scope.feeds = data.Opml || [];
				$scope.numfeeds = 0;
				$scope.stories = [];
				$scope.unreadStories = {};
				$scope.last = 0;
				$scope.xmlurls = {};
				$scope.icons = data.Icons;
				$scope.opts = data.Options ? JSON.parse(data.Options) : $scope.opts;

				var loadStories = function(feed) {
					$scope.numfeeds++;
					$scope.xmlurls[feed.XmlUrl] = feed;
					var stories = data.Stories[feed.XmlUrl] || [];
					for(var i = 0; i < stories.length; i++) {
						$scope.procStory(feed.XmlUrl, stories[i], false);
						if ($scope.last < stories[i].Date) {
							$scope.last = stories[i].Date;
						}
						$scope.stories.push(stories[i]);
						$scope.unreadStories[stories[i].guid] = true;
					}
				};

				for(var i = 0; i < $scope.feeds.length; i++) {
					var f = $scope.feeds[i];

					if (f.XmlUrl) {
						loadStories(f);
					} else if (f.Outline) {  // check for empty groups
						for(var j = 0; j < f.Outline.length; j++) {
							loadStories(f.Outline[j]);
							$scope.xmlurls[f.Outline[j].XmlUrl].folder = f.Title;
						}
					}
				}

				if (typeof cb === 'function') cb();
				$scope.loaded();
				$scope.updateUnread();
				$scope.updateStories();
				$scope.updateTitle();
				setTimeout($scope.applyGetFeed);
			})
			.error(function() {
				if (typeof cb === 'function') cb();
				$scope.loaded();
			});
	};

	$scope.updateTitle = function() {
		var ur = $scope.unread['all'] || 0;
		document.title = 'go read' + (ur != 0 ? ' (' + ur + ')' : '');
	};

	$scope.setCurrent = function(i, noClose, isClick, $event) {
		var middleClick = $event && $event.which == 2;
		if ($event && !middleClick) {
			$event.preventDefault();
		}
		if (!middleClick && !noClose && i == $scope.currentStory) {
			delete $scope.currentStory;
			return;
		}
		var story = $scope.dispStories[i];
		$scope.getContents(story);
		if (i > 0) {
			$scope.getContents($scope.dispStories[i - 1]);
		}
		if (i < $scope.dispStories.length - 2) {
			$scope.getContents($scope.dispStories[i + 1]);
		}
		if ($scope.currentStory != i) {
			setTimeout(function() {
				se = $('#storydiv' + i);
				var eTop = se.offset().top;
				if (!isClick || eTop < 0 || eTop > $('#story-list').height()) {
					se[0].scrollIntoView();
				}
			});
		}
		if (!middleClick) {
			$scope.currentStory = i;
		}
		$scope.markAllRead(story);
	};

	$scope.prev = function() {
		if ($scope.currentStory > 0) {
			$scope.setCurrent($scope.currentStory - 1);
		}
	};

	$scope.toggleHideEmpty = function() {
		$scope.opts.hideEmpty = !$scope.opts.hideEmpty;
		$scope.saveOpts();
	};

	$scope.shouldHideEmpty = function(f) {
		if (!$scope.opts.hideEmpty) return false;
		var cnt = f.Outline ? $scope.unread['folders'][f.Title] : $scope.unread['feeds'][f.XmlUrl];
		return cnt == 0;
	};

	$scope.next = function() {
		if ($scope.dispStories && typeof $scope.currentStory === 'undefined') {
			$scope.setCurrent(0);
		} else if ($scope.dispStories && $scope.currentStory < $scope.dispStories.length - 1) {
			$scope.setCurrent($scope.currentStory + 1);
		}
	};

	$scope.updateUnread = function() {
		$scope.unread = {
			'all': 0,
			'feeds': {},
			'folders': {}
		};

		for (var i = 0; i < $scope.feeds.length; i++) {
			var f = $scope.feeds[i];
			if (f.Outline) {
				$scope.unread['folders'][f.Title] = 0;
				for (var j = 0; j < f.Outline.length; j++) {
					$scope.unread['feeds'][f.Outline[j].XmlUrl] = 0;
				}
			} else {
				$scope.unread['feeds'][f.XmlUrl] = 0;
			}
		}

		for (var i = 0; i < $scope.stories.length; i++) {
			var s = $scope.stories[i];
			if ($scope.unreadStories[s.guid]) {
				$scope.unread['all']++;
				$scope.unread['feeds'][s.feed.XmlUrl]++;
				var folder = $scope.xmlurls[s.feed.XmlUrl].folder;
				if (folder) {
					$scope.unread['folders'][folder]++;
				}
			}
		}
		$scope.updateUnreadCurrent();
	};

	$scope.updateUnreadCurrent = function() {
		if ($scope.activeFeed) $scope.unread.current = $scope.unread.feeds[$scope.activeFeed];
		else if ($scope.activeFolder) $scope.unread.current = $scope.unread.folders[$scope.activeFolder];
		else $scope.unread.current = $scope.unread.all;
	};

	$scope.markAllRead = function(story) {
		if (!$scope.dispStories.length) return;
		var ss = [];
		var checkStories = story ? [story] : $scope.dispStories;
		for (var i = 0; i < checkStories.length; i++) {
			var s = checkStories[i];
			if (!s.read) {
				if ($scope.opts.mode == 'unread') s.remove = true;
				s.read = true;
				ss.push({
					Feed: s.feed.XmlUrl,
					Story: s.Id
				});
				delete $scope.unreadStories[s.guid];
			}
		}
		$scope.http('POST', $('#mark-all-read').attr('data-url-read'), {
			stories: JSON.stringify(ss)
		});
		var unread = false;
		for (var i = $scope.stories.length - 1; i >= 0; i--) {
			if (!story && $scope.stories[i].remove) {
				$scope.stories.splice(i, 1);
			} else if (!$scope.stories[i].read) {
				unread = true;
			}
		}

		$scope.updateUnread();
		$scope.updateStories();
		$scope.updateTitle();

		if (!unread)
			$scope.http('POST', $('#mark-all-read').attr('data-url'), { last: $scope.last });
	};

	$scope.active = function() {
		if ($scope.activeFolder) return $scope.activeFolder;
		if ($scope.activeFeed) return $scope.xmlurls[$scope.activeFeed].Title;
		return 'all items';
	};

	$scope.nothing = function() {
		return $scope.loading == 0 && $scope.stories && !$scope.numfeeds && $scope.shown != 'about';
	};

	$scope.toggleNav = function() {
		$scope.opts.nav = !$scope.opts.nav;
		$scope.saveOpts();
	};

	$scope.toggleExpanded = function() {
		$scope.opts.expanded = !$scope.opts.expanded;
		$scope.saveOpts();
		$scope.applyGetFeed();
	};

	$scope.setExpanded = function(v) {
		$scope.opts.expanded = v;
		$scope.saveOpts();
		$scope.applyGetFeed();
	};

	$scope.navspan = function() {
		return $scope.opts.nav ? '' : 'no-nav';
	};

	$scope.navmargin = function() {
		return $scope.opts.nav ? {} : {'margin-left': '0'};
	};

	$scope.saveOpts = function() {
		$scope.http('POST', $('#story-list').attr('data-url-options'), {
			options: JSON.stringify($scope.opts)
		});
	};

	$scope.overContents = function(s) {
		if (typeof $scope.contents[s.guid] !== 'undefined') {
			return;
		}
		s.getTimeout = $timeout(function() {
			$scope.getContents(s);
		}, 250);
	};
	$scope.leaveContents = function(s) {
		if (s.getTimeout) {
			$timeout.cancel(s.getTimeout);
		}
	};

	$scope.toFetch = [];
	$scope.getContents = function(s) {
		if (typeof $scope.contents[s.guid] !== 'undefined') {
			return;
		}
		$scope.toFetch.push(s);
		if (!$scope.fetchPromise) {
			$scope.fetchPromise = $timeout($scope.fetchContents);
		}
	};

	$scope.fetchContents = function() {
		delete $scope.fetchPromise;
		if ($scope.toFetch.length == 0) {
			return;
		}
		var tofetch = $scope.toFetch;
		$scope.toFetch = [];
		var data = [];
		for (var i = 0; i < tofetch.length; i++) {
			$scope.contents[tofetch[i].guid] = '';
			data.push({
				Feed: tofetch[i].feed.XmlUrl,
				Story: tofetch[i].Id
			});
		}
		$http.post($('#mark-all-read').attr('data-url-contents'), data)
			.success(function(data) {
				var current = '';
				if ($scope.dispStories[$scope.currentStory]) {
					current = $scope.dispStories[$scope.currentStory].guid;
				}
				for (var i = 0; i < data.length; i++) {
					$scope.contents[tofetch[i].guid] = data[i];
				}
			});
	};

	$scope.setActiveFeed = function(feed) {
		delete $scope.activeFolder;
		$scope.activeFeed = feed;
		delete $scope.currentStory;
		$scope.updateStories();
		$scope.applyGetFeed();
		$scope.updateUnreadCurrent();
	};

	$scope.setActiveFolder = function(folder) {
		delete $scope.activeFeed;
		$scope.activeFolder = folder;
		delete $scope.currentStory;
		$scope.updateStories();
		$scope.applyGetFeed();
		$scope.updateUnreadCurrent();
	};

	$scope.setMode = function(mode) {
		$scope.opts.mode = mode;
		$scope.updateStories();
		$scope.applyGetFeed();
		$scope.saveOpts();
	};

	$scope.setSort = function(order) {
		$scope.opts.sort = order;
		$scope.updateStories();
		$scope.saveOpts();
	};

	$scope.updateStories = function() {
		$scope.dispStories = [];
		if ($scope.activeFolder) {
			for (var i = 0; i < $scope.stories.length; i++) {
				var s = $scope.stories[i];
				if ($scope.xmlurls[s.feed.XmlUrl].folder == $scope.activeFolder) {
					$scope.dispStories.push(s);
				}
			}
		} else if ($scope.activeFeed) {
			if ($scope.opts.mode != 'unread') {
				angular.forEach($scope.readStories[$scope.activeFeed], function(s) {
					if ($scope.unreadStories[s.guid]) {
						s.read = false;
					}
					$scope.dispStories.push(s);
				});
			} else {
				for (var i = 0; i < $scope.stories.length; i++) {
					var s = $scope.stories[i];
					if (s.feed.XmlUrl == $scope.activeFeed) {
						$scope.dispStories.push(s);
					}
				}
			}
		} else {
			$scope.dispStories = $scope.stories;
		}

		var swap = $scope.opts.sort == 'oldest'
		if (swap) {
			// turn off swap for all items mode on a feed
			if ($scope.activeFeed && $scope.opts.mode == 'all')
				swap = false;
		}
		$scope.dispStories.sort(function(_a, _b) {
			var a, b;
			if (!swap) {
				a = _a;
				b = _b;
			} else {
				a = _b;
				b = _a;
			}

			var d = b.Date - a.Date;
			if (!d)
				return a.guid.localeCompare(b.guid);
			return d;
		});
	};

	$scope.rename = function(feed) {
		var name = prompt('Rename to');
		if (!name) return;
		$scope.xmlurls[feed].Title = name;
		$scope.uploadOpml();
	};

	$scope.renameFolder = function(folder) {
		var name = prompt('Rename to');
		if (!name) return;
		for (var i = 0; i < $scope.feeds.length; i++) {
			var f = $scope.feeds[i];
			if (f.Outline && f.Title == folder) {
				f.Title = name;
				$scope.activeFolder = name;
				break;
			}
		}
		$scope.uploadOpml();
	};

	$scope.deleteFolder = function(folder) {
		if (!confirm('Delete ' + folder + ' and unsubscribe from all feeds in it?')) return;
		for (var i = 0; i < $scope.feeds.length; i++) {
			var f = $scope.feeds[i];
			if (f.Outline && f.Title == folder) {
				$scope.feeds.splice(i, 1);
				break;
			}
		}
		$scope.setActiveFeed();
		$scope.uploadOpml();
	};

	$scope.unsubscribe = function(feed) {
		if (!confirm('Unsubscribe from ' + $scope.xmlurls[feed].Title + '?')) return;
		for (var i = 0; i < $scope.feeds.length; i++) {
			var f = $scope.feeds[i];
			if (f.Outline) {
				for (var j = 0; j < f.Outline.length; j++) {
					if (f.Outline[j].XmlUrl == feed) {
						f.Outline.splice(j, 1);
						break;
					}
				}
				if (!f.Outline.length) {
					$scope.feeds.splice(i, 1);
					break;
				}
			}
			if (f.XmlUrl == feed) {
				$scope.feeds.splice(i, 1);
				break;
			}
		}
		$scope.stories = $scope.stories.filter(function(e) {
			return e.feed.XmlUrl != feed;
		});
		$scope.setActiveFeed();
		$scope.uploadOpml();
		$scope.updateUnread();
	};

	$scope.moveFeed = function(url, folder) {
		var feed;
		var found = false;
		for (var i = $scope.feeds.length - 1; i >= 0; i--) {
			var f = $scope.feeds[i];
			if (f.Outline) {
				for (var j = 0; j < f.Outline.length; j++) {
					var o = f.Outline[j];
					if (o.XmlUrl == url) {
						if (f.Title == folder)
							return;
						feed = f.Outline[j];
						f.Outline.splice(j, 1);
						if (!f.Outline.length)
							$scope.feeds.splice(i, 1);
						break;
					}
				}
				if (f.Title == folder)
					found = true;
			} else if (f.XmlUrl == url) {
				if (!folder)
					return;
				feed = f;
				$scope.feeds.splice(i, 1)
			}
		}
		if (!feed) return;
		if (!folder) {
			$scope.feeds.push(feed);
		} else {
			if (!found) {
				$scope.feeds.push({
					Outline: [],
					Title: folder
				});
			}
			for (var i = 0; i < $scope.feeds.length; i++) {
				var f = $scope.feeds[i];
				if (f.Outline && f.Title == folder) {
					$scope.feeds[i].Outline.push(feed);
				}
			}
		}
		$scope.uploadOpml();
	};

	$scope.moveFeedNew = function(url) {
		var folder = prompt('New folder name');
		if (!folder) return;
		$scope.moveFeed(url, folder);
	};

	$scope.uploadOpml = function() {
		$scope.http('POST', $('#story-list').attr('data-url-upload'), {
			opml: JSON.stringify($scope.feeds)
		});
	};

	var sl = $('#story-list');
	$scope.readStories = {};
	$scope.cursors = {};
	$scope.fetching = {};
	$scope.getFeed = function() {
		var f = $scope.activeFeed;
		if (!f || $scope.fetching[f]) return;
		if ($scope.dispStories.length != 0) {
			var sh = sl[0].scrollHeight;
			var h = sl.height();
			var st = sl.scrollTop()
			if (sh - (st + h) > 200) {
				return;
			}
		}
		$scope.http('GET', sl.attr('data-url-get-feed') + '?' + $.param({
			f: f,
			c: $scope.cursors[f] || ''
		})).success(function (data) {
			if (!data || !data.Stories) return
			delete $scope.fetching[f]
			$scope.cursors[$scope.activeFeed] = data.Cursor;
			if (!$scope.readStories[f])
				$scope.readStories[f] = [];
			for (var i = 0; i < data.Stories.length; i++) {
				$scope.procStory(f, data.Stories[i], true);
				$scope.readStories[f].push(data.Stories[i]);
			}
			$scope.updateStories();
			$scope.applyGetFeed();
		});
		$scope.fetching[f] = true;
	};

	$scope.applyGetFeed = function() {
		if ($scope.opts.mode == 'all') {
			$scope.getFeed();
		}
		if ($scope.opts.expanded) {
			$scope.getVisibleContents();
		}
	};

	$scope.onScroll = _.debounce($scope.applyGetFeed, 300);
	sl.on('scroll', $scope.onScroll);
	$window.onscroll = $scope.onScroll;
	$window.onresize = $scope.onScroll;

	$scope.getVisibleContents = function() {
		var h = sl.height();
		var st = sl.scrollTop();
		var b = st + h + 200;
		var fetched = 0;
		for (var i = 0; i < $scope.dispStories.length && fetched < 10; i++) {
			var s = $scope.dispStories[i];
			if ($scope.contents[s.guid]) continue;
			var sd = $('#storydiv' + i);
			if (!sd.length) continue;
			var sdt = sd.position().top;
			var sdb = sdt + sd.height();
			if (sdt < b && sdb > 0) {
				fetched += 1;
				$scope.getContents(s);
			}
		}
	};

	$scope.setAddSubscription = function() {
		$scope.shown = 'add-subscription';
		// need to wait for the keypress to finish before focusing
		setTimeout(function() {
			$('#add-subscription-form input').focus();
		});
	};

	var shortcuts = $('#shortcuts');
	Mousetrap.bind('?', function() {
		shortcuts.modal('toggle');
	});
	Mousetrap.bind('esc', function() {
		shortcuts.modal('hide');
	});
	Mousetrap.bind('r', function() {
		if ($scope.nouser) {
			return;
		}
		$scope.$apply($scope.refresh());
	});
	Mousetrap.bind(['j', 'n', 'space'], function() {
		$scope.$apply('next()');
	});
	Mousetrap.bind(['k', 'p', 'shift+space'], function() {
		$scope.$apply('prev()');
	});
	Mousetrap.bind('v', function() {
		if ($scope.dispStories[$scope.currentStory]) {
			window.open($scope.dispStories[$scope.currentStory].Link);
		}
	});
	Mousetrap.bind('shift+a', function() {
		if ($scope.nouser) {
			return;
		}
		$scope.$apply($scope.markAllRead());
	});
	Mousetrap.bind('a', function() {
		if ($scope.nouser) {
			return;
		}
		$scope.$apply("setAddSubscription()");
	});
	Mousetrap.bind('g a', function() {
		if ($scope.nouser) {
			return;
		}
		$scope.$apply("shown = 'feeds'; setActiveFeed();");
	});
	Mousetrap.bind('u', function() {
		$scope.$apply("toggleNav()");
	});
	Mousetrap.bind('1', function() {
		$scope.$apply("setExpanded(true)");
	});
	Mousetrap.bind('2', function() {
		$scope.$apply("setExpanded(false)");
	});

	$scope.showMessage = function(m) {
		$('#message-list').text(m);
		$('#messages').modal('show');
	};

	$scope.setYesterday = function() {
		var d = new Date();
		d.setDate(d.getDate() - 1);
		$scope.http('POST', $('#mark-all-read').attr('data-url'), { last: d.valueOf() });
	};
});
