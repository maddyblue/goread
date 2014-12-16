$('.dropdown-toggle').dropdown();

if ($('#messages').attr('data-show')) {
	$('#messages').modal();
}

var goReadAppModule = angular.module('goReadApp', ['ui.sortable'])
	.config(function($sceDelegateProvider) {
		$sceDelegateProvider.resourceUrlWhitelist(['.*']);
	})
	.filter('encodeURI', function() {
		return encodeURIComponent;
	});
goReadAppModule.controller('GoreadCtrl', function($scope, $http, $timeout, $window, $sce) {
	$scope.loading = 0;
	$scope.feeds = {};
	$scope.stories = {};

	$scope.opts = {
		folderClose: {},
		nav: true,
		expanded: false,
		mode: 'unread',
		sort: 'newest',
		hideEmpty: false,
		scrollRead: false
	};

	$scope.sortableOptions = {
		stop: function() {
			$scope.uploadOpml();
		}
	};

	$scope.importOpml = function() {
		$scope.shown = 'feeds';
		$scope.loading++;
		$('#import-opml-form').ajaxForm({
			clearForm: true,
			error: function(jqXHR, textStatus, errorThrown) {
				$scope.showMessage(jqXHR.responseText);
			},
			success: function() {
				$scope.loaded();
				$scope.showMessage("OPML import is happening."
					+ " It can take a minute. Don't reorganize your feeds"
					+ " until it's completed importing."
					+ " Refresh to see its progress.");
			}
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

	$scope.addSubscription = function(e) {
		if (!$scope.addFeedUrl) {
			return false;
		}
		var btn = $(e.target);
		btn.button('loading')
		$scope.loading++;
		var f = $('#add-subscription-form');
		$scope.http('POST', f.attr('data-url'), {
			url: $scope.addFeedUrl
		}).then(function() {
			$scope.addFeedUrl = '';
			btn.button('reset')
			// I think this is needed due to the datastore's eventual consistency.
			// Without the delay we only get the feed data with no story data.
			$timeout(function() {
				$scope.refresh()
					.finally(function() {
						$scope.loaded();
						$scope.setActive('feed', _.last($scope.feeds).XmlUrl);
					});
			}, 250);
		}, function(data) {
			if (data.data) {
				alert(data.data);
			}
			$scope.loaded();
			btn.button('reset')
		});
	};

	$scope.procStory = function(xmlurl, story, read) {
		story.guid = xmlurl + '|' + story.Id;
		if ($scope.stories[story.guid]) return;
		story.read = !!read;
		story.feed = $scope.feeds[xmlurl];
		if (!story.Title) {
			story.Title = '(title unknown)';
		}
		var today = new Date().toDateString();
		var d = new Date(story.Date * 1000);
		story.dispdate = moment(d).format(d.toDateString() == today ? "h:mm a" : "MMM D, YYYY");
		story.canUnread = story.Created >= $scope.unreadDate;
		$scope.stories[story.guid] = story;
	};

	$scope.update = function() {
		$scope.updateFolders();
		$scope.updateCounts();
		$scope.updateStories();
	};

	$scope.updateCounts = function() {
		$scope.updateUnread();
		$scope.updateTitle();
	};

	$scope.updateFolders = function() {
		_.each($scope.opml, function(f) {
			if (f.Outline) {
				_.each(f.Outline, function(s) {
					var feed = $scope.feeds[s.XmlUrl];
					feed.folder = f.Title;
				});
			} else {
				var feed = $scope.feeds[f.XmlUrl];
				delete feed.folder;
			}
		});
	};

	$scope.refresh = function() {
		$scope.loading++;
		$scope.numfeeds = 0;
		$scope.shown = 'feeds';
		$scope.resetScroll();
		delete $scope.currentStory;
		var promise = $http.post($('#refresh').attr('data-url-feeds'))
			.success(function(data) {
				if (data.ErrorSubscription) {
					$timeout(function() {
						alert('Free trial ended. Please subscribe.');
					});
					$scope.shown = 'account';
					return;
				}
				$scope.unreadDate = data.UnreadDate;
				$scope.untilDays = data.UntilDate ? moment.unix(data.UntilDate).diff(moment(), 'days') : 0;
				$scope.opml = data.Opml || $scope.opml || [];
				_.each(data.Feeds, function(e) {
					e.Checked = moment(e.Checked).fromNow();
					e.NextUpdate = moment(e.NextUpdate).fromNow();
					$scope.feeds[e.Url] = e;
				});
				var mapFeed = function(o) {
					var f = $scope.feeds[o.XmlUrl];
					f.opml = o;
					_.each(o, function(value, key) {
						f[key] = value;
					});
				};
				for (var i = 0; i < $scope.opml.length; i++) {
					var o = $scope.opml[i];
					if (o.Outline) {
						_.each(o.Outline, mapFeed);
					} else if (o.XmlUrl) {
						mapFeed(o);
					} else {
						$scope.opml.splice(i, 1);
						i--;
					}
				}
				$scope.opts = data.Options ? JSON.parse(data.Options) : $scope.opts;
				$scope.trialRemaining = data.TrialRemaining;
				_.each(data.Stories, function(stories, feed) {
					$scope.numfeeds = 1;
					_.each(stories, function(story) {
						$scope.procStory(feed, story, false);
					});
				});
				_.each(data.Stars, function(s) {
					if ($scope.stories[s])
						$scope.stories[s].star = Date.now();
				});
			})
			.error(function() {
				alert('Error during refresh: try again.');
			})
			.finally(function() {
				$scope.loaded();
				$scope.update();
				$scope.resetLimit();
				setTimeout($scope.applyGetFeed);
			});
		return promise;
	};

	$scope.updateTitle = function() {
		var ur = $scope.unread['all'] || 0;
		document.title = 'go read' + (ur != 0 ? ' (' + ur + ')' : '');
	};

	/* options dictionary:
	 *  - collapse: can be true, false, or 'toggle'
	 *  - noMarkRead: don't mark as read
	 *  - noOpen: don't open or jump
	 *  - noScroll: don't scrollIntoView
	 *  - preventDefault: on middle/ctrl click, preventDefault
	 * ways to call this function:
	 *  - j/k
	 *  - n/p
	 *  - expanded mode click on item
	 *  - click on title: if open and list mode: collapse
	 *  - middle/ctrl click title: open new tab, mark read, don't open
	 *  - click right arrow: open new tab, mark read, don't open
	 *  - mark read on scroll: expanded mode only, don't jump
	 *  - NOT: o/enter: these have their own logic
	 */
	$scope.setCurrent = function(i, opts, $event) {
		opts = opts || {};
		var middleClick = $event && ($event.which == 2 || $event.ctrlKey);
		if (opts.preventDefault && $event && !middleClick) {
			$event.preventDefault();
		}
		var setCurrent = !opts.noOpen && !middleClick;
		var jumpStory = setCurrent && $scope.currentStory != i && !opts.noScroll;
		var collapse = opts.collapse; // undefined is falsy, so no collapse
		if ($scope.opts.expanded) {
			$scope.storyCollapse = false;
		} else if (collapse === 'toggle') {
			if ($scope.currentStory == i) {
				$scope.storyCollapse = !$scope.storyCollapse;
			} else {
				$scope.storyCollapse = false;
			}
		} else {
			$scope.storyCollapse = collapse ? true : false;
		}
		var markRead = !opts.noMarkRead && !$scope.storyCollapse;
		var story = $scope.dispStories[i];
		$scope.getContents(story);
		if (i > 0) {
			$scope.getContents($scope.dispStories[i - 1]);
		}
		if (i < $scope.dispStories.length - 2) {
			$scope.getContents($scope.dispStories[i + 1]);
		}
		if (i == $scope.dispLimit - 1) {
			$scope.loadNextPage();
		}
		if (jumpStory) {
			$timeout(function() {
				var se = $('#storydiv' + i);
				setTimeout(function() { se[0].scrollIntoView(); });
			});
		}
		if (setCurrent) {
			$scope.currentStory = i;
		}
		if (markRead) {
			$scope.markAllRead(story);
		}
	};

	$scope.prev = function(opts) {
		if ($scope.currentStory > 0) {
			$scope.setCurrent($scope.currentStory - 1, opts);
		}
	};

	$scope.toggleHideEmpty = function() {
		$scope.opts.hideEmpty = !$scope.opts.hideEmpty;
		$scope.saveOpts();
	};

	$scope.toggleScrollRead = function() {
		$scope.opts.scrollRead = !$scope.opts.scrollRead;
		$scope.saveOpts();
	};

	$scope.shouldHideEmpty = function(f) {
		if (!$scope.opts.hideEmpty) return false;
		var cnt = f.Outline ? $scope.unread['folders'][f.Title] : $scope.unread['feeds'][f.XmlUrl];
		return cnt == 0;
	};

	$scope.next = function(page, opts) {
		if ($scope.dispStories && typeof $scope.currentStory === 'undefined') {
			$scope.setCurrent(0, opts);
			return;
		}
		if (page) {
			var sl = $('#story-list');
			var sd = $('#storydiv' + $scope.currentStory);
			var sdh = sd.height();
			var sdt = sd.position().top;
			var slh = sl.height();
			if (sdt + sdh > slh) {
				var slt = sl.scrollTop();
				sl.scrollTop(slt + slh - 20);
				return;
			}
		}
		if ($scope.dispStories && $scope.currentStory < $scope.dispStories.length - 1) {
			$scope.setCurrent($scope.currentStory + 1, opts);
		}
	};

	$scope.updateUnread = function() {
		$scope.unread = {
			'all': 0,
			'feeds': {},
			'folders': {}
		};

		_.each($scope.opml, function(f) {
			if (f.Outline) {
				$scope.unread['folders'][f.Title] = 0;
				_.each(f.Outline, function(subf) {
					$scope.unread['feeds'][subf.XmlUrl] = 0;
				});
			} else {
				$scope.unread['feeds'][f.XmlUrl] = 0;
			}
		});

		_.each($scope.stories, function(s) {
			if (!s.read) {
				$scope.unread['all']++;
				$scope.unread['feeds'][s.feed.XmlUrl]++;
				var folder = s.feed.folder;
				if (folder) {
					$scope.unread['folders'][folder]++;
				}
			}
		});
		$scope.updateUnreadCurrent();
	};

	$scope.updateUnreadCurrent = function() {
		if ($scope.activeFeed) $scope.unread.current = $scope.unread.feeds[$scope.activeFeed];
		else if ($scope.activeFolder) $scope.unread.current = $scope.unread.folders[$scope.activeFolder];
		else $scope.unread.current = $scope.unread.all;
	};

	$scope.markReadStories = [];
	$scope.markAllRead = function(story) {
		if (!$scope.dispStories.length) return;
		var checkStories = story ? [story] : $scope.dispStories;
		_.each(checkStories, function(s) {
			if (!s.read) {
				s.read = true;
				$scope.markReadStories.push({
					Feed: s.feed.XmlUrl,
					Story: s.Id
				});
			}
		});
		$scope.sendReadStories();
		$scope.updateCounts();
	};

	$scope.markUnread = function(s) {
		s.read = !s.read;
		var attr = s.read ? '' : 'un';
		$scope.http('POST', $('#mark-all-read').attr('data-url-' + attr + 'read'), {
			feed: s.feed.XmlUrl,
			story: s.Id
		});
		$scope.updateCounts();
	};

	$scope.sendReadStories = _.debounce(function() {
		var ss = $scope.markReadStories;
		$scope.markReadStories = [];
		if (ss.length > 0) {
			$http.post($('#mark-all-read').attr('data-url-read'), ss);
			$scope.$apply();
		}
	}, 500);

	$scope.active = function() {
		if ($scope.activeFolder) return $scope.activeFolder;
		if ($scope.activeFeed) return $scope.feeds[$scope.activeFeed].Title;
		if ($scope.activeStar) return 'starred items';
		return 'all items';
	};

	$scope.nothing = function() {
		return $scope.loading == 0 && $scope.stories && !$scope.numfeeds && $scope.shown != 'about' && $scope.shown != 'account' && $scope.shown != 'feed-history';
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

	var prevOpts;
	$scope.saveOpts = _.debounce(function() {
		var opts = JSON.stringify($scope.opts);
		if (opts == prevOpts) return;
		prevOpts = opts;
		$scope.http('POST', $('#story-list').attr('data-url-options'), {
			options: opts
		});
		$scope.$apply();
	}, 1000);

	$scope.overContents = function(s) {
		if (typeof s.contents !== 'undefined') {
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
		if (typeof s.contents !== 'undefined') return;
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
		_.each(tofetch, function(s) {
			s.contents = '';
			data.push({
				Feed: s.feed.XmlUrl,
				Story: s.Id
			});
		});
		$http.post($('#mark-all-read').attr('data-url-contents'), data)
			.success(function(data) {
				_.each(data, function(d, i) {
					var div = $('<div>' + d + '</div>');
					$('a', div).attr('target', '_blank');
					tofetch[i].contents = $sce.trustAsHtml(div.html());
				});
			});
	};

	var limitInc = 20;
	$scope.dispLimit = limitInc;
	$scope.resetLimit = function() {
		$scope.dispLimit = limitInc;
		$scope.checkLoadNextPage();
	}

	$scope.setActive = function(type, value) {
		delete $scope.activeFeed;
		delete $scope.activeFolder;
		delete $scope.activeStar;
		delete $scope.activeAll;
		if (type == 'feed') $scope.activeFeed = value;
		if (type == 'folder') $scope.activeFolder = value;
		if (type == 'star') $scope.activeStar = true;
		if (!type) $scope.activeAll = true;
		delete $scope.currentStory;
		$scope.updateStories();
		$scope.applyGetFeed();
		$scope.updateUnreadCurrent();
		$scope.resetScroll();
		$scope.resetLimit();
	};

	$scope.resetScroll = function() {
		$('#story-list').scrollTop(0);
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
		$scope.resetScroll();
		$scope.resetLimit();
	};

	$scope.updateStories = function() {
		$scope.dispStories = [];
		_.each($scope.stories, function(s) {
			if ($scope.opts.mode == 'unread' && s.read) {
				return;
			} else if ($scope.activeFolder) {
				if (s.feed.folder != $scope.activeFolder) {
					return;
				}
			} else if ($scope.activeFeed) {
				if (s.feed.XmlUrl != $scope.activeFeed) {
					return;
				}
			} else if ($scope.activeStar) {
				if (!s.star) {
					return;
				}
			}
			$scope.dispStories.push(s);
		});

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
		var name = prompt('Rename to', $scope.feeds[feed].Title);
		if (!name) return;
		$scope.feeds[feed].Title = name;
		$scope.feeds[feed].opml.Title = name;
		$scope.uploadOpml();
	};

	$scope.renameFolder = function(folder) {
		var name = prompt('Rename to', folder);
		if (!name) return;
		var src, dst;
		for (var i = 0; i < $scope.opml.length; i++) {
			var f = $scope.opml[i];
			if (f.Outline) {
				if (f.Title == folder) src = f;
				else if (f.Title == name) dst = f;
			}
		}
		if (!dst) {
			src.Title = name;
		} else {
			dst.Outline.push.apply(dst.Outline, src.Outline);
			var idx = $scope.feeds.indexOf(src);
			$scope.feeds.splice(idx, 1);
		}
		$scope.activeFolder = name;
		$scope.uploadOpml();
		$scope.update();
	};

	$scope.deleteFolder = function(folder) {
		if (!confirm('Delete ' + folder + ' and unsubscribe from all feeds in it?')) return;
		for (var i = 0; i < $scope.opml.length; i++) {
			var f = $scope.opml[i];
			if (f.Outline && f.Title == folder) {
				$scope.opml.splice(i, 1);
				break;
			}
		}
		$scope.setActive();
		$scope.uploadOpml();
		$scope.update();
	};

	$scope.unsubscribe = function(feed) {
		if (!confirm('Unsubscribe from ' + $scope.feeds[feed].Title + ' (' + feed + ')?')) return;
		for (var i = 0; i < $scope.opml.length; i++) {
			var f = $scope.opml[i];
			if (f.Outline) {
				for (var j = 0; j < f.Outline.length; j++) {
					if (f.Outline[j].XmlUrl == feed) {
						f.Outline.splice(j, 1);
						break;
					}
				}
				if (!f.Outline.length) {
					$scope.opml.splice(i, 1);
					break;
				}
			}
			if (f.XmlUrl == feed) {
				$scope.opml.splice(i, 1);
				break;
			}
		}
		angular.forEach($scope.stories, function(v, k) {
			if (v.feed.XmlUrl == feed) {
				delete $scope.stories[k];
			}
		});
		$scope.setActive();
		$scope.uploadOpml();
		$scope.update();
	};

	$scope.moveFeed = function(url, folder) {
		var feed;
		var found = false;
		for (var i = $scope.opml.length - 1; i >= 0; i--) {
			var f = $scope.opml[i];
			if (f.Outline) {
				for (var j = 0; j < f.Outline.length; j++) {
					var o = f.Outline[j];
					if (o.XmlUrl == url) {
						if (f.Title == folder)
							return;
						feed = f.Outline[j];
						f.Outline.splice(j, 1);
						if (!f.Outline.length)
							$scope.opml.splice(i, 1);
						break;
					}
				}
				if (f.Title == folder)
					found = true;
			} else if (f.XmlUrl == url) {
				if (!folder)
					return;
				feed = f;
				$scope.opml.splice(i, 1)
			}
		}
		if (!feed) return;
		if (!folder) {
			$scope.opml.push(feed);
		} else {
			if (!found) {
				$scope.opml.push({
					Outline: [],
					Title: folder
				});
			}
			for (var i = 0; i < $scope.opml.length; i++) {
				var f = $scope.opml[i];
				if (f.Outline && f.Title == folder) {
					$scope.opml[i].Outline.push(feed);
				}
			}
		}
		$scope.uploadOpml();
		$scope.update();
	};

	$scope.moveFeedNew = function(url) {
		var folder = prompt('New folder name');
		if (!folder) return;
		$scope.moveFeed(url, folder);
	};

	var prevOpml;
	$scope.uploadOpml = _.debounce(function() {
		var opml = JSON.stringify($scope.opml);
		if (opml == prevOpml) return;
		prevOpml = opml;
		$scope.http('POST', $('#story-list').attr('data-url-upload'), {
			opml: opml
		});
		$scope.$apply();
	}, 1000);

	var sl = $('#story-list');
	$scope.cursors = {};
	$scope.fetching = {};
	$scope.getFeed = function() {
		if ($scope.activeFeed) {
			var f = $scope.activeFeed;
			if ($scope.fetching[f]) return;
			$scope.fetching[f] = true;
			var url = sl.attr('data-url-get-feed') + '?' + $.param({
				f: f,
				c: $scope.cursors[f] || ''
			});
			var success = function (data) {
				if (!data.Stories) return;
				delete $scope.fetching[f];
				$scope.cursors[f] = data.Cursor;
				_.each(data.Stories, function(s) {
					$scope.procStory(f, s, true);
				});
				_.each(data.Stars, function(s) {
					$scope.stories[s].star = Date.now();
				});
			};
		} else if ($scope.activeStar) {
			if ($scope.fetching['stars']) return;
			$scope.fetching['stars'] = true;
			var url = sl.attr('data-url-get-stars') + '?' + $.param({
				c: $scope.cursors['stars'] || ''
			});
			var success = function(data) {
				if (!data.Stories) return;
				delete $scope.fetching['stars'];
				$scope.cursors['stars'] = data.Cursor;
				_.each(data.Feeds, function(f) {
					if (!$scope.feeds[f.Url]) {
						$scope.feeds[f.Url] = f;
					}
				});
				_.each(data.Stories, function(stories, f) {
					_.each(stories, function(s) {
						$scope.procStory(f, s, true);
					});
				});
				_.each(data.Stars, function(v, k) {
					$scope.stories[k].star = v * 1000;
				});
			};
		} else {
			return;
		}
		if ($scope.dispStories.length != 0) {
			var sh = sl[0].scrollHeight;
			var h = sl.height();
			var st = sl.scrollTop()
			if (sh - (st + h) > 200) {
				return;
			}
		}
		$scope.http('GET', url).success(function(data) {
			success(data);
			$scope.updateStories();
			$scope.checkLoadNextPage();
			$scope.applyGetFeed();
		});
	};

	$scope.applyGetFeed = function() {
		if ($scope.opts.mode == 'all' || $scope.activeStar) {
			$scope.getFeed();
		}
		if ($scope.opts.expanded) {
			$scope.getVisibleContents();
		}
	};

	$scope.onScroll = _.debounce(function() {
		$scope.$apply(function() {
			$scope.applyGetFeed();
			$scope.scrollRead();
			$scope.collapsed = $(window).width() <= 768;
			$scope.checkLoadNextPage();
		});
	}, 100);

	$scope.checkLoadNextPage = function() {
		if(!sl.length) return;
		var sh = sl[0].scrollHeight;
		var h = Math.min(sl.height(), $(window).height());
		var st = Math.max(sl.scrollTop(), $(window).scrollTop());
		if (sh - (st + h) > 200) {
			return;
		}
		$scope.loadNextPage();
	};

	$scope.loadNextPage = function() {
		var max = $scope.dispStories.length;
		var len = $scope.dispLimit + limitInc;
		if (len > max)
			len = max;
		else if (len < max)
			$timeout($scope.checkLoadNextPage);
		$scope.dispLimit = len;
	};

	sl.on('scroll', $scope.onScroll);
	$window.onscroll = $scope.onScroll;
	$window.onresize = $scope.onScroll;

	$scope.scrollRead = function() {
		if (!$scope.opts.scrollRead || !$scope.opts.expanded) return;
		var sl = $('#story-list');
		var slh = sl.height();
		var sle = sl[0];
		if (sle.scrollHeight == sle.scrollTop + slh) {
			$scope.markAllRead();
			$scope.setCurrent($scope.dispStories.length - 1);
			return;
		}
		// find the first visible item
		for (var i = 0; i < $scope.dispStories.length; i++) {
			var sd = $('#storydiv' + i + ' .story-content');
			var sdt = sd.position().top;
			if (sdt >= 0) {
				// first item is long, second item is below window scroll
				if (sdt > slh) {
					i--;
				}
				for (var j = $scope.currentStory + 1; j < i; j++) {
					$scope.markAllRead($scope.dispStories[j]);
				}
				$scope.setCurrent(i, {noScroll: true});
				break;
			}
		}
	};

	$scope.getVisibleContents = function() {
		var h = sl.height();
		var st = sl.scrollTop();
		var b = st + h + 200;
		var fetched = 0;
		for (var i = 0; i < $scope.dispStories.length && fetched < 10; i++) {
			var s = $scope.dispStories[i];
			if ($scope.stories[s.guid].contents) continue;
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
			$('#add-subscription-form input[type="text"]').focus();
		});
	};

	$scope.clearFeeds = function() {
		if (!confirm('Remove all folders and subscriptions?')) return;
		$scope.feeds = {};
		$scope.stories = {};
		$scope.opml = [];
		$scope.setActive();
		$scope.uploadOpml();
		$scope.update();
	};

	$scope.deleteAccount = function() {
		if (!confirm('Delete your account?')) return;
		window.location.href = $('#delete-account').attr('data-url');
	};

	var checkoutLoaded = false;
	$scope.getAccount = function() {
		$scope.loadCheckout();
		$scope.shown = 'account';
		if ($scope.account) return;
		$http.post($('#account').attr('data-url-account'))
			.success(function(data) {
				$scope.account = data;
			});
	};

	$scope.loadCheckout = function(cb) {
		if (!checkoutLoaded) {
			$.getScript("https://checkout.stripe.com/v2/checkout.js", function() {
				checkoutLoaded = true;
				if (cb) cb();
			});
		} else {
			if (cb) cb();
		}
	};

	$scope.date = function(d) {
		var m = moment(d);
		if (!m.isValid()) return d;
		return m.format('D MMMM YYYY');
	};

	$scope.checkout = function(plan, desc, amount) {
		$scope.loadCheckout(function() {
			var token = function(res){
				var button = $('#button' + plan);
				button.button('loading');
				$scope.http('POST', $('#account').attr('data-url-charge'), {
					token: res.id,
					plan: plan
				})
					.success(function(data) {
						button.button('reset');
						$scope.accountType = 2;
						$scope.account = data;
					})
					.error(function(data) {
						button.button('reset');
						alert(data);
					});
				$scope.$apply();
			};
			StripeCheckout.open({
				key: $('#account').attr('data-stripe-key'),
				amount: amount,
				currency: 'usd',
				name: 'Go Read',
				description: desc,
				panelLabel: 'Subscribe for',
				token: token
			});
		});
	};

	$scope.unCheckout = function() {
		if (!confirm('Sure you want to unsubscribe?')) return;
		var button = $('#uncheckoutButton');
		button.button('loading');
		$http.post($('#account').attr('data-url-uncheckout'))
			.success(function() {
				delete $scope.account;
				$scope.accountType = 0;
				button.button('reset');
				alert('Unsubscribed');
			})
			.error(function(data) {
				button.button('reset');
				alert('Error');
			});
	};

	$scope.getFeedHistory = function() {
		$http.post($('#feed-history').attr('data-url'))
			.success(function(data) {
				$scope.shown = 'feed-history';
				$scope.feedHistory = [];
				data.reverse();
				for(var i = 0; i < data.length; i++) {
					var m = data[i];
					m = parseInt(m.substr(0, m.length - 6), 10);
					$scope.feedHistory.push({
						value: data[i],
						time: moment(m).format('MMMM Do YYYY, h:mm a')
					});
				}
			});
	};

	$scope.toggleStar = function(story) {
		if ($scope.stories[story.guid].star) {
			delete $scope.stories[story.guid].star;
		} else {
			$scope.stories[story.guid].star = Date.now();
		}
		$scope.http('POST',  $('#mark-all-read').attr('data-url-star'), {
			feed: story.feed.XmlUrl,
			story: story.Id,
			del: $scope.stories[story.guid].star ? '' : '1'
		});
	};

	$scope.encode = encodeURIComponent;

	$scope.shortcuts = $('#shortcuts');
	Mousetrap.bind('?', function() {
		$scope.shortcuts.modal('toggle');
		return false;
	});
	Mousetrap.bind('esc', function() {
		$scope.shortcuts.modal('hide');
		$('#messages').modal('hide');
		return false;
	});
	Mousetrap.bind('r', function() {
		if ($scope.nouser) {
			return;
		}
		$scope.refresh();
		$scope.$apply();
		return false;
	});
	Mousetrap.bind('n', function() {
		$scope.$apply(function() {
			$scope.next(null, {collapse: true});
		});
		return false;
	});
	Mousetrap.bind('j', function() {
		$scope.$apply(function() {
			$scope.next(null, {collapse: false});
		});
		return false;
	});
	Mousetrap.bind('space', function() {
		$scope.$apply('next(true)');
		return false;
	});
	Mousetrap.bind('p', function() {
		$scope.$apply(function() {
			$scope.prev({collapse: true});
		});
		return false;
	});
	Mousetrap.bind(['k', 'shift+space'], function() {
		$scope.$apply(function() {
			$scope.prev({collapse: false});
		});
		return false;
	});
	Mousetrap.bind('v', function() {
		if ($scope.dispStories[$scope.currentStory]) {
			window.open($scope.dispStories[$scope.currentStory].Link);
			return false;
		}
	});
	Mousetrap.bind('b', function() {
		var s = $scope.dispStories[$scope.currentStory];
		if (s) {
			var $link = document.createElement("a");
			$link.href = s.Link;
			var evt = document.createEvent("MouseEvents");
			evt.initMouseEvent("click", true, true, window, 0, 0, 0, 0, 0, true, false, false, true, 0, null);
			$link.dispatchEvent(evt);
			return false;
		}
	});
	Mousetrap.bind('shift+a', function() {
		if ($scope.nouser) {
			return;
		}
		$scope.$apply($scope.markAllRead());
		return false;
	});
	Mousetrap.bind('a', function() {
		if ($scope.nouser) {
			return;
		}
		$scope.$apply("setAddSubscription()");
		return false;
	});
	Mousetrap.bind('g a', function() {
		if ($scope.nouser) {
			return;
		}
		$scope.$apply("shown = 'feeds'; setActive();");
		return false;
	});
	Mousetrap.bind('u', function() {
		$scope.$apply("toggleNav()");
		return false;
	});
	Mousetrap.bind('1', function() {
		$scope.$apply("setExpanded(true)");
		return false;
	});
	Mousetrap.bind('2', function() {
		$scope.$apply("setExpanded(false)");
		return false;
	});
	Mousetrap.bind('m', function() {
		var s = $scope.dispStories[$scope.currentStory];
		if (s && s.canUnread) {
			$scope.$apply(function() {
				s.Unread = !s.Unread;
				$scope.markUnread(s);
			});
		}
		return false;
	});
	Mousetrap.bind(['o', 'enter'], function() {
		$scope.$apply(function() {
			$scope.storyCollapse = !$scope.storyCollapse;
			if (!$scope.storyCollapse) {
				$scope.markAllRead($scope.dispStories[$scope.currentStory]);
			}
		});
		return false;
	});

	$scope.registerHandler = function() {
		if (navigator && navigator.registerContentHandler) {
			navigator.registerContentHandler("application/vnd.mozilla.maybe.feed", "http://" + window.location.host + "/user/add-subscription?url=%s", "Go Read");
		}
	};

	$scope.showMessage = function(m) {
		$('#message-list').text(m);
		$('#messages').modal('show');
	};
});
