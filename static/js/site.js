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
		var url = $('#refresh').attr('data-url');
		$http.get(url)
			.success(function(data) {
				$scope.feeds = data;
			});
	};

	$scope.refresh();
}
