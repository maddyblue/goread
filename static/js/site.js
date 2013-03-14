$('.dropdown-toggle').dropdown();

function GoreadCtrl($scope) {
	$scope.shown = 'import-xml';

	$scope.importXml = function() {
		$('#import-xml-form').ajaxForm(function() {
		});
	};
}
