import javax.crypto.Cipher;
import javax.crypto.spec.IvParameterSpec;
import javax.crypto.spec.SecretKeySpec;

public class Aes256Ctr {

    public static byte[] decrypt(byte[] ciphertext, byte[] key, byte[] iv) throws Exception {
        Cipher cipher = Cipher.getInstance("AES/CTR/NoPadding");
        SecretKeySpec secretKey = new SecretKeySpec(key, "AES");
        IvParameterSpec ivSpec = new IvParameterSpec(iv);
        cipher.init(Cipher.DECRYPT_MODE, secretKey, ivSpec);
        return cipher.doFinal(ciphertext);
    }

    public static void main(String[] args) throws Exception {

        int[] intArray = {253, 72, 209, 96, 36};
        byte[] ciphertext = new byte[intArray.length];
        for (int i = 0; i < intArray.length; i++) {
            ciphertext[i] = (byte) intArray[i];
        }
//        byte[] ciphertext =  //byte[]{253,72,209,96,36]; // ciphertext to be decrypted
        byte[] key = "01234567012345670123456701234567".getBytes();// 256-bit key
        byte[] iv = "0123456701234567".getBytes();// initialization vector
        byte[] plaintext = decrypt(ciphertext, key, iv);
        System.out.println(new String(plaintext, "UTF-8"));
    }
}