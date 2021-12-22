usePlugin('@nomiclabs/buidler-truffle5');

module.exports = {
  defaultNetwork: 'local',
  networks: {
    local: {
      url: "http://localhost:8545",
      accounts: 
	      { mnemonic: "tray ripple elevator ramp insect butter top mouse old cinnamon panther chief"} ,
    }
  },
  solc: {
    version: '0.5.13',
    optimizer: {
      enabled: false
    },
    evmVersion: 'istanbul'
  },
  mocha: {
    timeout: 50000
  }
};
