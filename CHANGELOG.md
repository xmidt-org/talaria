# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/)
and this project adheres to [Semantic Versioning](http://semver.org/spec/v2.0.0.html).
## [Unreleased]
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
- Update webpa-common version with data race fix for a device's metadata claims getter [#134](https://github.com/xmidt-org/talaria/pull/134)

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

[Unreleased]: https://github.com/xmidt-org/talaria/compare/v0.5.4...HEAD
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

