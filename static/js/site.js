$('.dropdown-toggle').dropdown();

function GoreadCtrl($scope, $http) {
	$scope.importOpml = function() {
		$('#import-opml-form').ajaxForm(function() {
		});
	};

	$scope.addSubscription = function() {
		$('#add-subscription-form').ajaxForm(function() {
			$scope.refresh();
		});
	};

	$scope.refresh = function() {
		$http.get($('#refresh').attr('data-url-feeds'))
			.success(function(data) {
				$scope.feeds = data;
				$scope.stories = [];
				$scope.storyfeeds = {};
				for(var p in $scope.feeds) {
					var f = $scope.feeds[p];
					for(var i = 0; i < f.Stories.length; i++) {
						f.Stories[i].feed = f.Feed;
						var d = new Date(f.Stories[i].Date * 1000);
						f.Stories[i].dispdate = d.toDateString();
						$scope.stories.push(f.Stories[i]);
					}
				}
			});
	};

	$scope.refresh();
}
