# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/)
and this project adheres to [Semantic Versioning](http://semver.org/spec/v2.0.0.html).

## [Unreleased]
- Added RawAttributes interface. [#162](https://github.com/xmidt-org/talaria/pull/162)

## [v0.5.10]
### Fixed
- Add documentation around WRP source checking. [#154](https://github.com/xmidt-org/talaria/pull/154)
- Fixes from [this](https://github.com/xmidt-org/webpa-common/blob/main/CHANGELOG.md#v1108) version of webpa-common. [#156](https://github.com/xmidt-org/talaria/pull/156)


## [v0.5.9]
### Added
- Added `trust` label to hardware_model gauge. [#151](https://github.com/xmidt-org/talaria/pull/151)

### Fixed
- Fix device Metadata Trust() method float64 casting to int. [#151](https://github.com/xmidt-org/talaria/pull/151)

### Changed
- Add configurable check for the source of inbound (device => cloud service) WRP messages. [#151](https://github.com/xmidt-org/talaria/pull/151)
- Populate empty inbound WRP.content_type field with `application/octet-stream`. [#151](https://github.com/xmidt-org/talaria/pull/151)


## [v0.5.8]
- bumped webpa-common to fix consul last wait index and AllDatacenters [#147](https://github.com/xmidt-org/talaria/pull/147)

## [v0.5.7]
### Added
- New labels for the `hardware_model` gauge from webpa-common. [#146](https://github.com/xmidt-org/talaria/pull/146)

### Changed 
- Update references to the main branch. [#144](https://github.com/xmidt-org/talaria/pull/144)


## [v0.5.6]
- fixed xresolver failing to route traffic to caduceus using default http and https ports. [#143](https://github.com/xmidt-org/talaria/pull/143)

## [v0.5.5]
### Changed
- Query values from device claims without modifying map. [#137](https://github.com/xmidt-org/talaria/pull/137)


## [v0.5.4]
### Changed
- Remove header requirement for value that's not necessary [#135](https://github.com/xmidt-org/talaria/pull/135)

### Fixed
- Fixed consul xresolver watch config [#138](https://github.com/xmidt-org/talaria/pull/138)
- Fixed caduceus outbound sender via consul [#138](https://github.com/xmidt-org/talaria/pull/138)


## [v0.5.3]
### Fixed
- Update webpa-common version with data race fix for a device's metadata claims getter. [#134](https://github.com/xmidt-org/talaria/pull/134)

## [v0.5.2]
### Fixed
- Perform proper conversion of device trust integer value into a string. [#130](https://github.com/xmidt-org/talaria/pull/130)

## [v0.5.1]
### Fixed
- Service discovery metrics label includes correct service name for consul. [#127](https://github.com/xmidt-org/talaria/pull/127)
- Rehasher filters out service discovery events from services it wasn't configured with. [#127](https://github.com/xmidt-org/talaria/pull/127)

## [v0.5.0]
- Added deviceAccessCheck which includes partner ID checking [#117](https://github.com/xmidt-org/talaria/pull/117)

## [v0.4.0]
- Replace MessageHandler with WRPHandler [#121](https://github.com/xmidt-org/talaria/pull/121)
- adding docker automation [#122](https://github.com/xmidt-org/talaria/pull/122)

## [v0.3.0]
- add caduceus round robin logic through consul [#69](https://github.com/xmidt-org/talaria/pull/69)

## [v0.2.2]
- bumped webpa-common to v1.6.3 to make sessionID a first class citizen [#120](https://github.com/xmidt-org/talaria/pull/120)

## [v0.2.1]
- bumped webpa-common to v1.6.1 to fix crash [#118](https://github.com/xmidt-org/talaria/pull/118)

## [v0.2.0]
- convert from glide to go mod
- updated release pipeline to use travis [#113](https://github.com/xmidt-org/talaria/pull/113)
- added session id to event metadata [#117](https://github.com/xmidt-org/talaria/pull/117)
- bumped webpa-common to v1.6.0 [#117](https://github.com/xmidt-org/talaria/pull/117)

## [v0.1.3]
fixed build upload

## [v0.1.2]
Switching to new build process

## [v0.1.1] Tue Mar 28 2017 Weston Schmidt - 0.1.1
- initial creation

[Unreleased]: https://github.com/xmidt-org/talaria/compare/v0.5.10...HEAD
[v0.5.10]: https://github.com/xmidt-org/talaria/compare/v0.5.9...v0.5.10
[v0.5.9]: https://github.com/xmidt-org/talaria/compare/v0.5.8...v0.5.9
[v0.5.8]: https://github.com/xmidt-org/talaria/compare/v0.5.7...v0.5.8
[v0.5.7]: https://github.com/xmidt-org/talaria/compare/v0.5.6...v0.5.7
[v0.5.6]: https://github.com/xmidt-org/talaria/compare/v0.5.5...v0.5.6
[v0.5.5]: https://github.com/xmidt-org/talaria/compare/v0.5.4...v0.5.5
[v0.5.4]: https://github.com/xmidt-org/talaria/compare/v0.5.3...v0.5.4
[v0.5.3]: https://github.com/xmidt-org/talaria/compare/v0.5.2...v0.5.3
[v0.5.2]: https://github.com/xmidt-org/talaria/compare/v0.5.1...v0.5.2
[v0.5.1]: https://github.com/xmidt-org/talaria/compare/v0.5.0...v0.5.1
[v0.5.0]: https://github.com/xmidt-org/talaria/compare/v0.4.0...v0.5.0
[v0.4.0]: https://github.com/xmidt-org/talaria/compare/v0.3.0...v0.4.0
[v0.3.0]: https://github.com/xmidt-org/talaria/compare/v0.2.2...v0.3.0
[v0.2.2]: https://github.com/xmidt-org/talaria/compare/v0.2.1...v0.2.2
[v0.2.1]: https://github.com/xmidt-org/talaria/compare/v0.2.0...v0.2.1
[v0.2.0]: https://github.com/xmidt-org/talaria/compare/v0.1.3...v0.2.0
[v0.1.3]: https://github.com/xmidt-org/talaria/compare/v0.1.2...v0.1.3
[v0.1.2]: https://github.com/xmidt-org/talaria/compare/v0.1.1...v0.1.2
[v0.1.1]: https://github.com/xmidt-org/talaria/compare/v0.1.0...v0.1.1
