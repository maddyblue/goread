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
				for(var p in $scope.feeds) {
					$scope.numfeeds++;
					var f = $scope.feeds[p];
					if (!f.Stories)
						continue;
					$scope.unreadStories = {};
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

	var shortcuts = $('#shortcuts');
	Mousetrap.bind('?', function() {
		shortcuts.modal('toggle');
	});
	Mousetrap.bind('esc', function() {
		shortcuts.modal('hide');
	});
	Mousetrap.bind('r', function() {
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
		$scope.$apply($scope.markAllRead());
	});
	Mousetrap.bind('a', function() {
		$scope.$apply("shown = 'add-subscription'");
	Mousetrap.bind('g a', function() {
		$scope.$apply("shown = 'feeds'");
	});

		// need to wait for the keypress to finish before focusing
		setTimeout(function() {
			$('#add-subscription-form input').focus();
		}, 0);
	});

	$scope.refresh();
}
