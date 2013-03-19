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
			$scope.loaded();
		});
	};

	$scope.loaded = function() {
		$scope.loading--;
	};

	$scope.addSubscription = function() {
		$scope.shown = 'feeds';
		$scope.loading++;
		$('#add-subscription-form').ajaxForm(function() {
			$scope.refresh($scope.loaded);
		});
	};

	$scope.refresh = function(cb) {
		$scope.loading++;
		$http.get($('#refresh').attr('data-url-feeds'))
			.success(function(data) {
				$scope.feeds = data;
				$scope.stories = [];
				for(var p in $scope.feeds) {
					var f = $scope.feeds[p];
					if (!f.Stories)
						continue;
					$scope.unreadStories = {};
					for(var i = 0; i < f.Stories.length; i++) {
						f.Stories[i].feed = f.Feed;
						var d = new Date(f.Stories[i].Date * 1000);
						f.Stories[i].dispdate = d.toDateString();
						$scope.stories.push(f.Stories[i]);
						$scope.unreadStories[f.Stories[i].Id] = true;
					}
				}
				if (cb) cb();
				$scope.loaded();
			})
			.error(function() {
				if (cb) cb();
				$scope.loaded();
			});
	};

	$scope.setCurrent = function(s) {
		$scope.currentStory = s;
		$scope.markRead(s);
	};

	$scope.unread = function() {
		return countProperties($scope.unreadStories);
	};

	$scope.markRead = function(s) {
		delete $scope.unreadStories[s.Id];
		$http({
			method: 'POST',
			url: $('#mark-all-read').attr('data-url-read'),
			data: $.param({
				feed: s.feed.Url,
				story: s.Id
			}),
			headers: {'Content-Type': 'application/x-www-form-urlencoded'}
		});
	};

	$scope.markAllRead = function(s) {
		$scope.unreadStories = []
		$scope.stories = [];
		$http.post($('#mark-all-read').attr('data-url'));
	};

	$scope.refresh();
}
