import React, { Component } from 'react';
import ReactDOM from 'react-dom';
import idbKeyval from 'idb-keyval';

import MuiThemeProvider from 'material-ui/styles/MuiThemeProvider';

import AppBar from 'material-ui/AppBar';
import Dialog from 'material-ui/Dialog';
import IconButton from 'material-ui/IconButton';
import IconMenu from 'material-ui/IconMenu';
import FlatButton from 'material-ui/FlatButton';
import MenuItem from 'material-ui/MenuItem';
import MenuIcon from 'material-ui/svg-icons/navigation/menu';
import NavigationRefresh from 'material-ui/svg-icons/navigation/refresh';
import Snackbar from 'material-ui/Snackbar';
import TextField from 'material-ui/TextField';
import {Card, CardHeader, CardText, CardTitle} from 'material-ui/Card';

var appState = 'app-state';

var ADDR = process.env.NODE_ENV === 'production' ? location.origin : 'http://localhost:8080';

class App extends Component {
	state = {
		stories: [],
		read: {},
		snack: null,
	};

	login = () => {
		// Fetch something not cached by the service worker.
		fetch('/?nocache', {
			// Attempt to stop browser and network caching.
			cache: 'no-cache',
		})
		.then((resp) => {
			if (resp.ok) {
				window.location = ADDR + '/login/redirect?redirect=' + location.origin;
			} else {
				this.setState({snack: 'network error'});
			}
		}, () => {
			this.setState({snack: 'network error'});
		});
	}
	fetch = (path, init, nojson) => {
		var options = {
			method: 'POST',
			credentials: 'include',
			mode: 'cors',
		};
		Object.assign(options, init);
		var url = ADDR + path;
		return new Promise(
			(resolve, reject) => {
				fetch(url, options)
				.then(resp => {
					if (resp.url !== url) {
						this.login();
					}
					if (resp.ok) {
						if (nojson) {
							resolve(resp);
						} else {
							resp.json().then(resolve, reject);
						}
					} else {
						reject(resp.statusText);
					}
				})
				.catch(err => {
					this.login();
				});
			}
		);
	}
	componentDidMount = () => {
		idbKeyval.get(appState).then(s => {
			if (!s || !s.stories || !s.stories.length) {
				this.refresh();
			} else {
				this.setState(s);
			}
		});
	}
	componentWillUpdate = (nextProps, nextState) => {
		idbKeyval.set(appState, nextState);
	}
	refresh = () => {
		var that = this;
		this.setState({snack: 'refreshing...'});
		this.fetch('/user/list-feeds')
		.then(that.setStories)
		.then(() => {
			this.setState({snack: null});
		}, err => {
			debugger;
			this.setState({snack: err.toString()});
		});
	}
	setStories = data => {
		if (!data.Feeds) {
			return;
		}
		var sorted = [];
		var that = this;
		var feeds = {};
		data.Feeds.forEach((feed, idx) => {
			feeds[feed.Url] = feed;
		});
		function assignFeed(feedURL) {
			data.Stories[feedURL].forEach(story => {
				var uid = feedURL + ' ' + story.Id;
				if (that.state.read[uid]) {
					return;
				}
				story.uid = uid;
				story.feed = feeds[feedURL];
				story.unread = true;
				story.storyid = {
					Feed: feedURL,
					Story: story.Id,
				};
				sorted.push(story);
			});
		}
		for (var feedURL in data.Stories) {
			if(data.Stories.hasOwnProperty(feedURL)) {
				assignFeed(feedURL);
			}
		}
		sorted.sort(function(a, b) {
			return b.Date - a.Date;
		});
		var missing = [];
		sorted.forEach(story => {
			missing.push({
				Feed: story.feed.Url,
				Story: story.Id,
			});
		});
		if (!missing.length) {
			return;
		}
		this.fetch('/user/get-contents', {
			body: JSON.stringify(missing),
		})
		.then(data => {
			sorted.forEach((story, idx) => {
				story.contents = data[idx];
			});
			that.setState({
				stories: sorted,
				read: {},
			});
		});
	}
	expand = (story, expanded) => {
		if (!expanded) {
			this.setState({expanded: null});
			return;
		}
		var read = this.state.read;
		read[story.uid] = true;
		this.setState({
			expanded: story.uid,
			read: read,
		});
		this.fetch('/user/mark-read', {
			body: JSON.stringify([story.storyid]),
		}, true);
	}
	componentDidUpdate = () => {
		if (this._expanded) {
			var n = ReactDOM.findDOMNode(this._expanded);
			n.scrollIntoView();
		}
	}
	closeSnack = () => {
		this.setState({snack: null});
	}
	toDesktop() {
		document.cookie = 'goread-desktop=desktop; max-age=31536000';
		location.reload();
	}
	subscribe = () => {
		this.setState({openSubscribeFeed: true});
	}
	closeSubscribeFeed = () => {
		this.setState({openSubscribeFeed: false});
	}
	subscribeFeed = () => {
		var form = new FormData();
		form.append('url', this.state.subscribeFeed);
		this.fetch('/user/add-subscription', {
			body: form,
		}, true)
		.then(() => {
			this.refresh();
		}, data => {
			this.setState({snack: data});
		});
		this.setState({
			openSubscribeFeed: false,
			subscribeFeed: '',
		});
	}
	setSubscribeFeed = (event) => {
		this.setState({
			subscribeFeed: event.target.value,
		});
	}
	render() {
		this._expanded = null;
		return (
			<MuiThemeProvider>
				<div>
					<AppBar
						title="go read"
						iconElementLeft={
							<IconMenu
								iconButtonElement={
									<IconButton><MenuIcon color={'white'} /></IconButton>
								}
							>
								<MenuItem
									primaryText="subscribe to feed"
									onTouchTap={this.subscribe}
								/>
								<MenuItem
									primaryText="desktop site"
									onTouchTap={this.toDesktop}
								/>
							</IconMenu>
						}
						iconElementRight={
							<IconButton
								onClick={this.refresh}
							>
								<NavigationRefresh />
							</IconButton>
						}
					/>
					{this.state.stories.map(s => {
						var expanded = s.uid === this.state.expanded;
						return <Card
							key={s.uid}
							expanded={expanded}
							onExpandChange={expanded => { this.expand(s, expanded); }}
							>
							<CardHeader
								title={s.Title}
								subtitle={s.Summary}
								avatar={s.feed.Image}
								actAsExpander={true}
								showExpandableButton={true}
								titleStyle={{
									fontWeight: this.state.read[s.uid] ? '' : 'bold',
								}}
							/>
							<CardTitle
								title={<a href={s.Link} target="_blank">{s.Title}</a>}
								subtitle={<span>from <a href={s.feed.Link} target="_blank">{s.feed.Title}</a></span>}
								ref={c => {if(expanded) this._expanded = c}}
								expandable={true}
							/>
							<CardText
								expandable={true}
								dangerouslySetInnerHTML={{__html: s.contents}}
							></CardText>
						</Card>
					})}
					<Snackbar
						open={!!this.state.snack}
						message={this.state.snack || ''}
						autoHideDuration={4000}
						onRequestClose={this.closeSnack}
					/>
					<Dialog
						title="subscribe to feed"
						type="url"
						actions={[
							<FlatButton
								label="cancel"
								primary={true}
								onTouchTap={this.closeSubscribeFeed}
							/>,
							<FlatButton
								label="subscribe"
								primary={true}
								keyboardFocused={true}
								onTouchTap={this.subscribeFeed}
							/>,
						]}
						open={this.state.openSubscribeFeed || false}
						onRequestClose={this.closeSubscribeFeed}
					>
						<TextField
							hintText="feed URL"
							value={this.state.subscribeFeed || ''}
							onChange={this.setSubscribeFeed}
							fullWidth={true}
						/>
					</Dialog>
				</div>
			</MuiThemeProvider>
		);
	}
}

export default App;
