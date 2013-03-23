$('.dropdown-toggle').dropdown();

function countProperties(obj) {
	var count = 0;
	for(var prop in obj) {
		if(obj.hasOwnProperty(prop))
			++count;
	}
	return count;
}

function GoreadCtrl($scope, $http) {
	$scope.loading = 0;
	$scope.contents = {};

	$scope.importOpml = function() {
		$scope.shown = 'feeds';
		$scope.loading++;
		$('#import-opml-form').ajaxForm(function() {
			$('#import-opml-form')[0].reset();
			$scope.loaded();
		});
	};

	$scope.loaded = function() {
		$scope.loading--;
	};

	$scope.http = function(method, url, data) {
		return $http({
			method: method,
			url: url,
			data: $.param(data),
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
			$scope.refresh($scope.loaded);
		}, function(data) {
			alert(data.data);
			$scope.loading--;
		});
	};

	$scope.refresh = function(cb) {
		$scope.loading++;
		$scope.shown = 'feeds';
		delete $scope.currentStory;
		$http.get($('#refresh').attr('data-url-feeds'))
			.success(function(data) {
				$scope.feeds = data;
				$scope.numfeeds = 0;
				$scope.stories = [];
				$scope.unreadStories = {};
				for(var p in $scope.feeds) {
					$scope.numfeeds++;
					var f = $scope.feeds[p];
					if (!f.Stories)
						continue;
					for(var i = 0; i < f.Stories.length; i++) {
						f.Stories[i].feed = f.Feed;
						var d = new Date(f.Stories[i].Date * 1000);
						f.Stories[i].dispdate = d.toDateString();
						f.Stories[i].read = false;
						$scope.stories.push(f.Stories[i]);
						$scope.unreadStories[f.Stories[i].Id] = true;
					}
					$scope.stories.sort(function(a, b) {
						return b.Date - a.Date;
					});
				}
				if (typeof cb === 'function') cb();
				$scope.loaded();
				$scope.getContents();
			})
			.error(function() {
				if (typeof cb === 'function') cb();
				$scope.loaded();
			});
	};

	$scope.setCurrent = function(i) {
		$scope.currentStory = i;
		$scope.markRead($scope.stories[i]);
	};
	$scope.prev = function() {
		if ($scope.currentStory > 0) {
			$scope.$apply('setCurrent(currentStory - 1)');
		}
	};
	$scope.next = function() {
		if ($scope.stories && typeof $scope.currentStory === 'undefined') {
			$scope.$apply('setCurrent(0)');
		} else if ($scope.stories && $scope.currentStory < $scope.stories.length - 2) {
			$scope.$apply('setCurrent(currentStory + 1)');
		}
	};

	$scope.unread = function() {
		return countProperties($scope.unreadStories);
	};

	$scope.markRead = function(s) {
		if ($scope.unreadStories[s.Id]) {
			delete $scope.unreadStories[s.Id];
			s.read = true;
			$scope.http('POST', $('#mark-all-read').attr('data-url-read'), {
				feed: s.feed.Url,
				story: s.Id
			});
		}
	};

	$scope.markAllRead = function(s) {
		$scope.unreadStories = {};
		$scope.stories = [];
		$http.post($('#mark-all-read').attr('data-url'));
	};

	$scope.nothing = function() {
		return $scope.loading == 0 && $scope.stories && !$scope.numfeeds;
	};

	$scope.toggleNav = function() {
		$scope.nav = !$scope.nav;
	}
	$scope.navspan = function() {
		return 'span' + ($scope.nav ? '10' : '12');
	};
	$scope.navmargin = function() {
		return $scope.nav ? {} : {'margin-left': '0'};
	};

	$scope.storyClass = function(s) {
		var o = {};
		o['nocontent'] = $scope.contents[s.Id] === undefined;
		return o;
	};

	$scope.getContents = function() {
		var docViewTop = $(window).scrollTop();
		var docViewBottom = docViewTop + $(window).height();

		var stories = $('.nocontent');
		if (stories.length == 0) return;
		var onscreen = [];
		$.each(stories, function(i, v) {
			v = $(v);
			var elemTop = v.offset().top;
			var elemBottom = elemTop + v.height();
			if ((elemTop <= docViewBottom) && (elemBottom >= docViewTop)) {
				var s = $scope.stories[v.attr('data-idx')];
				onscreen.push({
					Feed: s.feed.Url,
					Story: s.Id,
				});
				// mark this as fetched so a subsequent scroll doesn't re fetch
				$scope.contents[s.Id] = '';
			}
		});
		if (onscreen.length == 0) return;
		$http.post($('#mark-all-read').attr('data-url-contents'), onscreen)
			.success(function(data) {
				for (var k in data) {
					$scope.contents[k] = data[k];
				}
			});
	};
	window.onscroll = function() {
		$scope.$apply('getContents()');
	};
	// todo: switch this cheating to a directive
	setTimeout(function() {
		$scope.$apply('getContents()');
	}, 100);

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
	Mousetrap.bind('j', $scope.next);
	Mousetrap.bind('k', $scope.prev);
	Mousetrap.bind('v', function() {
		if ($scope.stories[$scope.currentStory]) {
			window.open($scope.stories[$scope.currentStory].Link);
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
		$scope.$apply("shown = 'add-subscription'");

		// need to wait for the keypress to finish before focusing
		setTimeout(function() {
			$('#add-subscription-form input').focus();
		}, 0);
	});
	Mousetrap.bind('g a', function() {
		if ($scope.nouser) {
			return;
		}
		$scope.$apply("shown = 'feeds'");
	});
	Mousetrap.bind('u', function() {
		$scope.$apply("toggleNav()");
	});
}
