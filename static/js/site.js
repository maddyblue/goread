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
			});
		$http.get($('#refresh').attr('data-url-unread'))
			.success(function(data) {
				$scope.stories = data;
			});
	};

	$scope.refresh();
}
