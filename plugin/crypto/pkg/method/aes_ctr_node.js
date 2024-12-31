const crypto = require('crypto');

let key = Buffer.from('01234567012345670123456701234567', 'utf8');
console.log(key)
let iv = Buffer.from('0123456701234567', 'utf8');
console.log(iv)
let ciphertext = Buffer.from([253,72,209,96,36]);

const decipher = crypto.createDecipheriv('aes-256-ctr', key, iv);
const plaintext = decipher.update(ciphertext);
const finalPlaintext = decipher.final();
console.log(Buffer.concat([plaintext, finalPlaintext]).toString());

