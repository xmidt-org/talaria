# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/)
and this project adheres to [Semantic Versioning](http://semver.org/spec/v2.0.0.html).

## [Unreleased]


## [v0.6.11]
- Update configs and documentation for the introduction of OTLP in Candlelight [#282](https://github.com/xmidt-org/talaria/pull/282)
- update bascule config in docker config [#295](https://github.com/
xmidt-org/talaria/pull/295)
- remove/disable sat token checks from docker config [#297](https://github.com/xmidt-org/talaria/pull/297)

## [v0.6.10]
- Remove several unused build files and update the docker images to work. [#272](https://github.com/xmidt-org/talaria/pull/272)

## [v0.6.9]
- Fix the docker container so it has configuration in the right place.
- Fix the linter issues.
- Dependency update.


## [v0.6.8]
- JWT Migration [250](https://github.com/xmidt-org/talaria/issues/250)
  - updated to use clortho `Resolver`
  - updated to use clortho `metrics` & `logging`
- Update Config
  - Use [uber/zap](https://github.com/uber-go/zap) for clortho logging
  - Use [xmidt-org/sallust](https://github.com/xmidt-org/sallust) for the zap config unmarshalling 
  - Update auth config for clortho

## [v0.6.7]
- Dependency update, note vulnerabilities
    - github.com/hashicorp/consul/api v1.13.1 // indirect
        Wasn't able to find much info about this one besides the following dep vulns
        - golang.org/x/net
            - https://nvd.nist.gov/vuln/detail/CVE-2021-33194
            - https://nvd.nist.gov/vuln/detail/CVE-2021-31525
            - https://nvd.nist.gov/vuln/detail/CVE-2021-44716
    - Introduces new vuln https://www.mend.io/vulnerability-database/CVE-2022-29526
- QOS Ack implementation [#228](https://github.com/xmidt-org/talaria/issues/228) [#236](https://github.com/xmidt-org/talaria/pull/236)

## [v0.6.5]
- Bumped webpa-common to v2.0.6, fixing panic on send endpoint. [#229](https://github.com/xmidt-org/talaria/pull/229)

## [v0.6.4]
- Updated spec file and rpkg version macro to be able to choose when the 'v' is included in the version. [#199](https://github.com/xmidt-org/talaria/pull/199)
- Updated WRPHandler_Test to correspond with a bug fix in WRP-Go. [#206](https://github.com/xmidt-org/talaria/pull/206)
- Added /v2 support for service endpoints. [#225](https://github.com/xmidt-org/talaria/pull/225)

## [v0.6.3]
- Removed v1 webpa-common dependency that was accidentally added in v0.6.2. [#197](https://github.com/xmidt-org/talaria/pull/197)

## [v0.6.2]
- Fixed device endpoint bug, added info log for partner IDs and trust. [#196](https://github.com/xmidt-org/talaria/pull/196)

## [v0.6.1]
- Added v2 compatible device endpoint for older devices connecting. [#195](https://github.com/xmidt-org/talaria/pull/195)

## [v0.6.0]
- Updated api version in url to v3 to indicate breaking changes in response codes when an invalid auth is sent.  This change was made in an earlier release (v0.5.13). [#194](https://github.com/xmidt-org/talaria/pull/194)

## [v0.5.13]
- Changed Authkey from string splice to string [#188](https://github.com/xmidt-org/talaria/pull/188)
- Changed Content-type from "json" to "application/json"[#189](https://github.com/xmidt-org/talaria/pull/189)
- Removed jwt lib that's no longer maintained. [#187](https://github.com/xmidt-org/talaria/pull/187)
- Fixed reading of 'requestTimeout' config variable. [#187](https://github.com/xmidt-org/talaria/pull/187)
- Modify consul registration address value in spruce config file so rehasher would work as expected. [#190](https://github.com/xmidt-org/talaria/pull/190) thanks to @Sachin4403 

## [v0.5.12]
- Prevent Authorization header from getting logged. [#181](https://github.com/xmidt-org/talaria/pull/181)
- Add way to modify deviceAccessCheck type through docker-compose env var. [#184](https://github.com/xmidt-org/talaria/pull/184)

## [v0.5.11]
- Migrate to github actions, normalize analysis tools, Dockerfiles and Makefiles. [#166](https://github.com/xmidt-org/talaria/pull/166)
- Clarify comments of options in config file. [#176](https://github.com/xmidt-org/talaria/pull/176)
- Add OpenTelemetry tracing feature. [#178](https://github.com/xmidt-org/talaria/pull/178) thanks to @utsavbatra5

### Added
- Added RawAttributes interface. [#162](https://github.com/xmidt-org/talaria/pull/162)
- Added abiliy to gate and drain by specific device metadata parameters. [#172](https://github.com/xmidt-org/talaria/pull/172)

### Changed
- Update buildtime format in Makefile to match RPM spec file. [#164](https://github.com/xmidt-org/talaria/pull/164)

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

[Unreleased]: https://github.com/xmidt-org/talaria/compare/v0.6.11...HEAD
[v0.6.11]: https://github.com/xmidt-org/talaria/compare/v0.6.10...v0.6.11
[v0.6.10]: https://github.com/xmidt-org/talaria/compare/v0.6.9...v0.6.10
[v0.6.9]: https://github.com/xmidt-org/talaria/compare/v0.6.8...v0.6.9
[v0.6.8]: https://github.com/xmidt-org/talaria/compare/v0.6.7...v0.6.8
[v0.6.7]: https://github.com/xmidt-org/talaria/compare/v0.6.5...v0.6.7
[v0.6.5]: https://github.com/xmidt-org/talaria/compare/v0.6.4...v0.6.5
[v0.6.4]: https://github.com/xmidt-org/talaria/compare/v0.6.3...v0.6.4
[v0.6.3]: https://github.com/xmidt-org/talaria/compare/v0.6.2...v0.6.3
[v0.6.2]: https://github.com/xmidt-org/talaria/compare/v0.6.1...v0.6.2
[v0.6.1]: https://github.com/xmidt-org/talaria/compare/v0.6.0...v0.6.1
[v0.6.0]: https://github.com/xmidt-org/talaria/compare/v0.5.13...v0.6.0
[v0.5.13]: https://github.com/xmidt-org/talaria/compare/v0.5.12...v0.5.13
[v0.5.12]: https://github.com/xmidt-org/talaria/compare/v0.5.11...v0.5.12
[v0.5.11]: https://github.com/xmidt-org/talaria/compare/v0.5.10...v0.5.11
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
