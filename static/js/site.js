$('.dropdown-toggle').dropdown();

function countProperties(obj) {
	var count = 0;
	for(var prop in obj) {
		if(obj.hasOwnProperty(prop))
			++count;
	}
	return count;
}

function GoreadCtrl($scope, $http, $timeout) {
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
						f.Stories[i].guid = f.Feed.Url + '|' + f.Stories[i].Id;
						$scope.stories.push(f.Stories[i]);
						$scope.unreadStories[f.Stories[i].Id] = true;
					}
					$scope.stories.sort(function(a, b) {
						return b.Date - a.Date;
					});
				}
				if (typeof cb === 'function') cb();
				$scope.loaded();
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

	$scope.toFetch = [];
	$scope.getContents = function(s) {
		if (typeof $scope.contents[s.guid] !== 'undefined') {
			return;
		}
		$scope.toFetch.push(s);
		if (!$scope.fetchPromise) {
			// fetch this story immediately
			$scope.fetchContents();
			// and any others in a bit
			$scope.fetchPromise = $timeout($scope.fetchContents, 500);
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
				Feed: tofetch[i].feed.Url,
				Story: tofetch[i].Id
			});
		}
		$http.post($('#mark-all-read').attr('data-url-contents'), data)
			.success(function(data) {
				for (var i = 0; i < data.length; i++) {
					$scope.contents[tofetch[i].guid] = data[i];
				}
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
