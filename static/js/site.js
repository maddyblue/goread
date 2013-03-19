$('.dropdown-toggle').dropdown();

function GoreadCtrl($scope, $http) {
	$scope.shown = 'feeds';
	$scope.loading = 0;

	$scope.importOpml = function() {
		$('#import-opml-form').ajaxForm(function() {
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
		$http.get($('#refresh').attr('data-url-feeds'))
			.success(function(data) {
				$scope.feeds = data;
				$scope.stories = [];
				for(var p in $scope.feeds) {
					var f = $scope.feeds[p];
					if (!f.Stories)
						continue;
					for(var i = 0; i < f.Stories.length; i++) {
						f.Stories[i].feed = f.Feed;
						var d = new Date(f.Stories[i].Date * 1000);
						f.Stories[i].dispdate = d.toDateString();
						$scope.stories.push(f.Stories[i]);
					}
					$scope.unread = f.Stories.length;
					if (cb) cb();
				}
			});
	};

	$scope.setCurrent = function(s) {
		$scope.currentStory = s;
	};

	$scope.refresh();
}
