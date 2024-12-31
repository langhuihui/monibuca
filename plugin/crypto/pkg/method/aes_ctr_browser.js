
var aesjs = require('aes-js');
let ciphertext = Uint8Array.from([253, 72, 209, 96, 36]);

let key = aesjs.utils.utf8.toBytes('01234567012345670123456701234567');
console.log(key)

let iv = aesjs.utils.utf8.toBytes('0123456701234567');
console.log(iv)

var aesCtr = new aesjs.ModeOfOperation.ctr(key, new aesjs.Counter(iv));
var decryptedBytes = aesCtr.decrypt(ciphertext);
console.log(decryptedBytes)
console.log(aesjs.utils.utf8.fromBytes(decryptedBytes))
