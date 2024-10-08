package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
	"github.com/spf13/viper"

	"github.com/quicsec/quicsec/operations/log"
)

const QuicsecPrefix string = "QUICSEC"

type JSONLoader struct {
	config *Config
	once   sync.Once
}

func (j *JSONLoader) SetDefaultConfig() {
	j.config = &Config{
		ServiceConf: ServiceConf{
			Mtls: MtlsConfig{
				InsecSkipVerify: false,
				MtlsEnabled:     false,
			},
		},
		Log: LogConfigs{
			LogFileEnabled:       false,
			AccessLogFileEnabled: false,
			Debug:                true,
			Path:                 "",
		},
		Http: HttpConfigs{
			Access: AccessConfigs{
				Path: "",
			},
		},
		Quic: QuicConfigs{
			Debug: QuicDebugConfigs{
				SecretFilePathEnabled: false,
				SecretFilePath:        "",
				QlogEnabled:           false,
				QlogDirPath:           "",
			},
		},
		Metrics: MetricsConfigs{
			Enable:      true,
			BindEnabled: false,
			BindPort:    8080,
		},
		Certs: CertificatesConfigs{
			CaPath:   "certs/ca.pem",
			CertPath: "certs/cert.pem",
			KeyPath:  "certs/cert.key",
		},
	}
}

func (j *JSONLoader) Load() {
	j.once.Do(func() {
		var confLogger logr.Logger

		j.SetDefaultConfig()

		// read QUICSEC_CORE_CONFIG before viper init
		path, configFile, configCorePath := setupCoreConfig()

		viper.AddConfigPath(path)
		viper.SetConfigName(configFile) // Register config file name (no extension)
		viper.SetConfigType("json")     // Look for specific type

		viper.SetEnvPrefix(QuicsecPrefix)
		viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

		viper.AutomaticEnv()

		// defaults
		viper.SetDefault("serviceconf.mtls.insec_skip_verify", j.config.ServiceConf.Mtls.InsecSkipVerify) // QUICSEC_SERVICECONF_MTLS_INSEC_SKIP_VERIFY
		viper.SetDefault("serviceconf.mtls.client_cert", j.config.ServiceConf.Mtls.MtlsEnabled)           // QUICSEC_SERVICECONF_MTLS_CLIENT_CERT
		viper.SetDefault("log.debug", j.config.Log.Debug)                                                 // QUICSEC_LOG_DEBUG
		viper.SetDefault("log.path", j.config.Log.Path)                                                   // QUICSEC_LOG_PATH
		viper.SetDefault("http.access.path", j.config.Http.Access.Path)                                   // QUICSEC_HTTP_ACCESS_PATH
		viper.SetDefault("quic.debug.secret_path", j.config.Quic.Debug.SecretFilePath)                    // QUICSEC_QUIC_DEBUG_SECRET_PATH
		viper.SetDefault("quic.debug.qlog_path", j.config.Quic.Debug.QlogDirPath)                         // QUICSEC_QUIC_DEBUG_QLOG_PATH
		viper.SetDefault("metrics.enable", j.config.Metrics.Enable)                                       // QUICSEC_METRICS_ENABLE
		viper.SetDefault("metrics.bind_port", j.config.Metrics.BindPort)                                  // QUICSEC_METRICS_BIND_PORT
		viper.SetDefault("certs.ca_path", j.config.Certs.CaPath)                                          // QUICSEC_CERTS_CA_PATH
		viper.SetDefault("certs.key_path", j.config.Certs.KeyPath)                                        // QUICSEC_CERTS_KEY_PATH
		viper.SetDefault("certs.cert_path", j.config.Certs.CaPath)                                        // QUICSEC_CERTS_CERT_PATH

		if err := viper.ReadInConfig(); err != nil {
			fmt.Println("config: error reading config file: " + err.Error())
		} else {
			viper.WatchConfig()
			viper.OnConfigChange(func(e fsnotify.Event) {
				j.loadServiceConfig()
				confLogger.V(log.DebugLevel).Info("service config has changed...")
			})
		}

		if err := j.loadServiceConfig(); err != nil {
			fmt.Println("failed to load service config")
			panic(err.Error())
		}

		for _, key := range viper.AllKeys() {
			envKey := strings.ToUpper(QuicsecPrefix + "_" + strings.ReplaceAll(key, ".", "_"))
			err := viper.BindEnv(key, envKey)
			if err != nil {
				fmt.Println("config: unable to bind env: " + err.Error())
			}
		}

		// log into file
		if j.config.Log.Path != "" {
			j.config.Log.LogFileEnabled = true
		} else {
			j.config.Log.LogFileEnabled = false
		}

		log.InitLoggerLogr(j.config.Log.Debug, j.config.Log.Path)

		log.InitLoggerRequest(j.config.Log.Debug, j.config.Http.Access.Path)

		confLogger = log.LoggerLgr.WithName(log.ConstConfigManager)
		confLogger.V(log.DebugLevel).Info("all environment variables loaded")
		confLogger.V(log.DebugLevel).Info("core config", "path", configCorePath)

		// pre shared secret
		if j.config.Quic.Debug.SecretFilePath != "" {
			j.config.Quic.Debug.SecretFilePathEnabled = true
		}
		// qlog dir
		if j.config.Quic.Debug.QlogDirPath != "" {
			j.config.Quic.Debug.QlogEnabled = true
		}
		// prometheus metrics http
		if j.config.Metrics.BindPort != 0 {
			j.config.Metrics.BindEnabled = true
		}
		// log http requests into file
		if j.config.Http.Access.Path != "" {
			j.config.Log.AccessLogFileEnabled = true
		} else {
			j.config.Log.AccessLogFileEnabled = false
		}

		confLogger.V(log.DebugLevel).Info("all configuration loaded")
	})
}

func (j *JSONLoader) GetConfig() *Config {
	return j.config
}

func (j *JSONLoader) loadServiceConfig() error {

	if err := viper.Unmarshal(j.config); err != nil {
		fmt.Println("config: unable to decode into struct: " + err.Error())
	}

	rawServiceConfs := viper.Get("service_conf")

	serviceConfs, ok := rawServiceConfs.([]interface{})
	if !ok {
		return fmt.Errorf("expected a slice of service configurations, got something else")
	}

	for _, rawServiceConfs := range serviceConfs {
		serviceConfigMap, ok := rawServiceConfs.(map[string]interface{})
		if !ok {
			return fmt.Errorf("service_conf item is not a map[string]interface{}")
		}

		var serviceConf ServiceConf
		for k, v := range serviceConfigMap {
			switch k {
			case "conf_selector":
				if confSelector, ok := v.(string); ok {
					serviceConf.ConfSelector = confSelector
				}
			case "policy":
				if policyMap, ok := v.(map[string]interface{}); ok {
					serviceConf.Policy = make(map[string]PolicyData)
					for policyKey, policyValue := range policyMap {
						if policyDataMap, ok := policyValue.(map[string]interface{}); ok {
							var policyData PolicyData
							//load authz config
							if authz, ok := policyDataMap["authz"].(string); ok {
								policyData.Authz = AuthzValue(authz)
							}
							//load  filter chain config
							if filtersMap, ok := policyDataMap["filters"].(map[string]interface{}); ok {
								//load waf filter config
								if wafConfig, ok := filtersMap["waf"].(map[string]interface{}); ok {
									if len(wafConfig) > 0 {
										wafConfigParsed, err := parseWafConfig(wafConfig)
										if err != nil {
											return err
										}
										policyData.FilterChain.Waf = wafConfigParsed
										policyData.FilterChain.FiltersAvb = append(policyData.FilterChain.FiltersAvb, "waf")
									}
								}
								// load ext_authz filter config
								if extAuthConfig, ok := filtersMap["ext_auth"].(map[string]interface{}); ok {
									if len(extAuthConfig) > 0 {
										extAuthConfigParsed, err := parseExtAuthConfig(extAuthConfig)
										if err != nil {
											return err
										}
										policyData.FilterChain.ExtAuth.Opa = extAuthConfigParsed
										policyData.FilterChain.FiltersAvb = append(policyData.FilterChain.FiltersAvb, "ext_auth")
									}
								}
								// load oauth2 filter config
								if oAuth2Config, ok := filtersMap["oauth2"].(map[string]interface{}); ok {
									if len(oAuth2Config) > 0 {
										oAuth2ConfigParsed, err := parseOAuth2Config(oAuth2Config)
										if err != nil {
											return err
										}
										policyData.FilterChain.Oauth2 = oAuth2ConfigParsed
										policyData.FilterChain.FiltersAvb = append(policyData.FilterChain.FiltersAvb, "oauth2")
									}
								}
							}
							serviceConf.Policy[policyKey] = policyData
						}
					}
				}
			case "mtls":
				if mtlsMap, ok := v.(map[string]interface{}); ok {
					var mtlsConfig MtlsConfig
					if insecSkipVerify, ok := mtlsMap["insec_skip_verify"].(bool); ok {
						sv := os.Getenv("QUICSEC_SERVICECONF_MTLS_INSEC_SKIP_VERIFY")
						if sv == ""{
							mtlsConfig.InsecSkipVerify = insecSkipVerify
						} else {
							mtlsConfig.InsecSkipVerify = parseBool(sv)
						}
					}

					if clientCert, ok := mtlsMap["client_cert"].(bool); ok {
						cc := os.Getenv("QUICSEC_SERVICECONF_MTLS_CLIENT_CERT")
						if cc == ""{
							mtlsConfig.MtlsEnabled = clientCert
						} else {
							mtlsConfig.MtlsEnabled = parseBool(cc)
						}
					}
					serviceConf.Mtls = mtlsConfig
				}
			}
		}
		if matchSelector(serviceConf.ConfSelector) {
			j.config.ServiceConf.ConfSelector = serviceConf.ConfSelector
			j.config.ServiceConf.Policy = serviceConf.Policy
			j.config.ServiceConf.Mtls.MtlsEnabled = serviceConf.Mtls.MtlsEnabled
			j.config.ServiceConf.Mtls.InsecSkipVerify = serviceConf.Mtls.InsecSkipVerify
			break
		}
	}

	return nil
}

func (j *JSONLoader) PrintCurretConfig() {
	json, err := json.MarshalIndent(j.config, "", "    ")
	if err != nil {
		fmt.Println("failed to marshal config to json error:", err.Error())
	}
	fmt.Println(string(json))
}

func setupCoreConfig() (string, string, string) {
	var dir string
	var file string
	var coreConfigFull string

	coreConfig := os.Getenv(QuicsecPrefix + "_CORE_CONFIG")

	if coreConfig != "" {
		coreConfigFull = coreConfig
		// Check if file already exists
		dir, file = filepath.Split(coreConfig)
		// remove "json" from the file
		file = strings.Split(file, ".")[0]
	} else {
		// default value ./config
		coreConfigFull = "./config.json"
		dir = "./"
		file = "config"
	}

	return dir, file, coreConfigFull
}

func matchSelector(selector string) bool {
	/*nowadays we use local network interfaces IPs to match the selector
	  we should choose other types like instance IDs or UUIDs*/
	ips, err := getCurrentIPs()
	if err != nil {
		panic("failed to get ips from netwrok interfaces")
	}

	sIp := net.ParseIP(selector)
	if sIp == nil {
		panic("failed to parse conf_selector as an IP adrress")
	}
	return matchIP(sIp, ips)
}

func getCurrentIPs() ([]net.IP, error) {
	var ips []net.IP

	// Fetch all network interfaces
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range interfaces {
		addresses, err := iface.Addrs()
		if err != nil {
			return nil, err
		}

		for _, addr := range addresses {
			switch v := addr.(type) {
			case *net.IPNet:
				ips = append(ips, v.IP)
			case *net.IPAddr:
				ips = append(ips, v.IP)
			}
		}
	}

	return ips, nil
}

func matchIP(ip net.IP, ips []net.IP) bool {
	for _, i := range ips {
		if ip.Equal(i) {
			return true
		}
	}
	return false
}

func parseBool(str string) (bool) {
	switch strings.ToLower(str) {
    case "true", "1":
        return true
    case "false", "0", "":
        return false
    default:
        return false
    }
}
