const String huamiServiceUUID = '0000fee0-0000-1000-8000-00805f9b34fb';
const String chunkedWriteUUID = '00000016-0000-3512-2118-0009af100700';
const String chunkedReadUUID  = '00000017-0000-3512-2118-0009af100700';
const String activityControlUUID = '00000004-0000-3512-2118-0009af100700';
const String activityDataUUID    = '00000005-0000-3512-2118-0009af100700';
const String liveHeartRateUUID   = '00002a37-0000-1000-8000-00805f9b34fb';

const List<String> knownTypeCodes = [
  '0x01', '0x05', '0x13', '0x25', '0x2E',
  '0x38', '0x3A', '0x3D', '0x48', '0x49',
];

const String liveHeartRateTypeCode = '0x2a37';
const String appDocsSubdir = 'heliolytics';
const String sessionsSubdir = 'sessions';
const String authKeyStorageKey = 'heliolytics.auth_key';
const int defaultFetchWindowHours = 48;
const int defaultListenDurationSec = 300;
const int scanTimeoutSec = 10;
const int chunkReceiveTimeoutSec = 5;
const int chunkRetryCount = 1;
